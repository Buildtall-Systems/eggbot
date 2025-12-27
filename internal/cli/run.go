package cli

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/buildtall-systems/eggbot/internal/commands"
	"github.com/buildtall-systems/eggbot/internal/config"
	"github.com/buildtall-systems/eggbot/internal/db"
	"github.com/buildtall-systems/eggbot/internal/dm"
	"github.com/buildtall-systems/eggbot/internal/fsm"
	"github.com/buildtall-systems/eggbot/internal/lightning"
	"github.com/buildtall-systems/eggbot/internal/nostr"
	"github.com/buildtall-systems/eggbot/internal/zaps"
	gonostr "github.com/nbd-wtf/go-nostr"
	"github.com/nbd-wtf/go-nostr/keyer"
	"github.com/nbd-wtf/go-nostr/nip04"
	"github.com/nbd-wtf/go-nostr/nip19"
	"github.com/nbd-wtf/go-nostr/nip59"
	"github.com/spf13/cobra"
)

var runCmd = &cobra.Command{
	Use:   "run",
	Short: "Start the eggbot service",
	Long:  `Start the eggbot Nostr bot service. Connects to relays and listens for DM commands and zap payments.`,
	RunE:  runBot,
}

func init() {
	rootCmd.AddCommand(runCmd)
}

func runBot(cmd *cobra.Command, args []string) error {
	// Load config with secrets
	cfg, err := config.LoadWithSecrets()
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	log.Printf("eggbot starting...")
	log.Printf("bot npub: %s", cfg.Nostr.BotNpub)
	log.Printf("relays: %v", cfg.Nostr.Relays)
	log.Printf("database: %s", cfg.Database.Path)

	// Create keyer for cryptographic operations (signing, encrypt/decrypt)
	kr, err := keyer.NewPlainKeySigner(cfg.Nostr.BotSecretHex)
	if err != nil {
		return fmt.Errorf("creating keyer: %w", err)
	}

	// Open database and run migrations
	database, err := db.Open(cfg.Database.Path)
	if err != nil {
		return fmt.Errorf("opening database: %w", err)
	}
	defer func() { _ = database.Close() }()

	if err := database.Migrate(); err != nil {
		return fmt.Errorf("running migrations: %w", err)
	}
	log.Printf("database ready")

	// Create context that cancels on shutdown signals
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle shutdown signals
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		sig := <-sigCh
		log.Printf("received signal %v, shutting down...", sig)
		cancel()
	}()

	// Get high water mark from database to filter old events
	highWaterMark, err := database.GetHighWaterMark()
	if err != nil {
		return fmt.Errorf("getting high water mark: %w", err)
	}
	if highWaterMark > 0 {
		hwmTime := time.Unix(highWaterMark, 0)
		log.Printf("high water mark: %s", hwmTime.Format("2006/01/02 15:04:05"))
	}

	// Create and connect relay manager
	relayMgr := nostr.NewRelayManager(cfg.Nostr.Relays, cfg.Nostr.BotPubkeyHex)
	if err := relayMgr.Connect(ctx, highWaterMark); err != nil {
		return fmt.Errorf("connecting to relays: %w", err)
	}
	defer relayMgr.Close()

	log.Printf("eggbot running, waiting for events...")

	// Initialize event processor FSM
	processorFSM := fsm.NewEventProcessorFSM()

	// Main event loop
	for {
		select {
		case <-ctx.Done():
			log.Printf("shutting down...")
			return nil

		case event := <-relayMgr.DMEvents():
			if event == nil {
				continue
			}
			log.Printf("received DM event: %s (kind:%d)", event.ID, event.Kind)
			eventTs := int64(event.CreatedAt)

			// Transition FSM to processing DM state
			if err := processorFSM.Event(ctx, fsm.ProcessorEventDMReceived); err != nil {
				log.Printf("FSM error on DM received: %v", err)
				processorFSM.Reset()
				continue
			}

			isNew, err := database.TryProcess(event.ID, event.Kind, eventTs)
			if err != nil {
				log.Printf("dedup check failed: %v", err)
				continue
			}
			if !isNew {
				log.Printf("duplicate event %s, skipping", event.ID)
				continue
			}

			// Decrypt DM based on kind
			var senderPubkey, messageContent string
			var incomingProtocol dm.DMProtocol

			switch event.Kind {
			case gonostr.KindEncryptedDirectMessage: // NIP-04 legacy DM
				incomingProtocol = dm.ProtocolNIP04
				// Compute shared secret and decrypt
				sharedSecret, err := nip04.ComputeSharedSecret(event.PubKey, cfg.Nostr.BotSecretHex)
				if err != nil {
					log.Printf("failed to compute shared secret: %v", err)
					_ = database.SetHighWaterMark(eventTs)
					continue
				}
				messageContent, err = nip04.Decrypt(event.Content, sharedSecret)
				if err != nil {
					log.Printf("failed to decrypt NIP-04 DM: %v", err)
					_ = database.SetHighWaterMark(eventTs)
					continue
				}
				senderPubkey = event.PubKey

			case gonostr.KindGiftWrap: // NIP-17 gift-wrapped DM
				incomingProtocol = dm.ProtocolNIP17
				rumor, err := nip59.GiftUnwrap(*event, func(pubkey, ciphertext string) (string, error) {
					return kr.Decrypt(ctx, ciphertext, pubkey)
				})
				if err != nil {
					log.Printf("failed to unwrap DM: %v", err)
					_ = database.SetHighWaterMark(eventTs)
					continue
				}
				senderPubkey = rumor.PubKey
				messageContent = rumor.Content

			default:
				log.Printf("unexpected DM kind: %d", event.Kind)
				_ = database.SetHighWaterMark(eventTs)
				continue
			}

			// Convert sender hex pubkey to npub for display
			senderNpub, _ := nip19.EncodePublicKey(senderPubkey)
			log.Printf("DM from %s: %s", senderNpub, messageContent)

			// Check for admin broadcast command (special syntax, handled before normal parsing)
			if broadcastMsg, isBroadcast := parseBroadcast(messageContent); isBroadcast {
				if !commands.IsAdmin(senderNpub, cfg.Admins) {
					sendResponse(ctx, kr, relayMgr, cfg.Nostr.BotSecretHex, cfg.Nostr.BotPubkeyHex,
						senderPubkey, "Permission denied: broadcast requires admin privileges", incomingProtocol)
					_ = database.SetHighWaterMark(eventTs)
					continue
				}
				if broadcastMsg == "" {
					sendResponse(ctx, kr, relayMgr, cfg.Nostr.BotSecretHex, cfg.Nostr.BotPubkeyHex,
						senderPubkey, "Usage: message customers: <your message>", incomingProtocol)
					_ = database.SetHighWaterMark(eventTs)
					continue
				}

				log.Printf("admin %s broadcasting: %s", senderNpub, broadcastMsg)
				sent, failed := broadcastToCustomers(ctx, kr, relayMgr, cfg, database, broadcastMsg)

				summary := fmt.Sprintf("Broadcast sent to %d customers", sent)
				if failed > 0 {
					summary += fmt.Sprintf(" (%d failed)", failed)
				}
				sendResponse(ctx, kr, relayMgr, cfg.Nostr.BotSecretHex, cfg.Nostr.BotPubkeyHex,
					senderPubkey, summary, incomingProtocol)
				_ = database.SetHighWaterMark(eventTs)
				continue
			}

			// Parse command from message
			parsedCmd := commands.Parse(messageContent)
			if parsedCmd == nil {
				log.Printf("empty message, ignoring")
				_ = database.SetHighWaterMark(eventTs)
				continue
			}

			if !parsedCmd.IsValid() {
				log.Printf("unknown command: %s", parsedCmd.Name)
				sendResponse(ctx, kr, relayMgr, cfg.Nostr.BotSecretHex, cfg.Nostr.BotPubkeyHex, senderPubkey,
					fmt.Sprintf("Unknown command: %s. Send 'help' for available commands.", parsedCmd.Name), incomingProtocol)
				_ = database.SetHighWaterMark(eventTs)
				continue
			}

			// Check permissions
			if err := commands.CanExecute(ctx, database.DB, parsedCmd, senderNpub, cfg.Admins); err != nil {
				log.Printf("permission denied for %s: %v", senderNpub, err)
				sendResponse(ctx, kr, relayMgr, cfg.Nostr.BotSecretHex, cfg.Nostr.BotPubkeyHex, senderPubkey,
					fmt.Sprintf("Permission denied: %v", err), incomingProtocol)
				_ = database.SetHighWaterMark(eventTs)
				continue
			}

			log.Printf("executing command: %s %v", parsedCmd.Name, parsedCmd.Args)

			// Transition FSM to command processed state
			if err := processorFSM.Event(ctx, fsm.ProcessorEventCommandProcessed); err != nil {
				log.Printf("FSM error on command processed: %v", err)
				processorFSM.Reset()
				_ = database.SetHighWaterMark(eventTs)
				continue
			}

			// Execute the command
			lnClient := lightning.NewClient()
			execCfg := commands.ExecuteConfig{
				SatsPerHalfDozen: cfg.Pricing.SatsPerHalfDozen,
				Admins:           cfg.Admins,
				LightningAddress: cfg.Lightning.LightningAddress,
				BotNpub:          cfg.Nostr.BotNpub,
				LightningClient:  lnClient,
			}
			result := commands.Execute(ctx, database, parsedCmd, senderNpub, execCfg)

			// Check for errors and transition FSM if needed
			if result.Error != nil {
				if err := processorFSM.Event(ctx, fsm.ProcessorEventError); err != nil {
					log.Printf("FSM error on command error: %v", err)
				}
				log.Printf("command error: %v", result.Error)
				responseMsg := fmt.Sprintf("Error: %v", result.Error)
				sendResponse(ctx, kr, relayMgr, cfg.Nostr.BotSecretHex, cfg.Nostr.BotPubkeyHex, senderPubkey, responseMsg, incomingProtocol)
				processorFSM.Reset()
				_ = database.SetHighWaterMark(eventTs)
				continue
			}

			// Transition FSM to sending response state
			if err := processorFSM.Event(ctx, fsm.ProcessorEventResponseSent); err != nil {
				log.Printf("FSM error on response sent: %v", err)
				processorFSM.Reset()
				_ = database.SetHighWaterMark(eventTs)
				continue
			}

			log.Printf("command result: %s", result.Message)
			sendResponse(ctx, kr, relayMgr, cfg.Nostr.BotSecretHex, cfg.Nostr.BotPubkeyHex, senderPubkey, result.Message, incomingProtocol)

			// Notify admins of new orders (just the summary, not payment details)
			if parsedCmd.Name == commands.CmdOrder && result.Error == nil {
				orderSummary := strings.SplitN(result.Message, "\n", 2)[0]
				adminMsg := fmt.Sprintf("📥 New order from %s:\n%s", senderNpub, orderSummary)
				notifyAdmins(ctx, kr, relayMgr, cfg, adminMsg)
			}

			// Check for inventory notifications after commands that may increase inventory
			if parsedCmd.Name == commands.CmdInventory || parsedCmd.Name == commands.CmdCancel {
				checkInventoryNotifications(ctx, kr, relayMgr, cfg, database)
			}

			// Reset FSM to idle after DM processing completes
			processorFSM.Reset()
			_ = database.SetHighWaterMark(eventTs)

		case event := <-relayMgr.ZapEvents():
			if event == nil {
				continue
			}
			log.Printf("received zap event: %s (kind:%d)", event.ID, event.Kind)
			eventTs := int64(event.CreatedAt)

			// Transition FSM to processing zap state
			if err := processorFSM.Event(ctx, fsm.ProcessorEventZapReceived); err != nil {
				log.Printf("FSM error on zap received: %v", err)
				processorFSM.Reset()
				continue
			}

			isNew, err := database.TryProcess(event.ID, event.Kind, eventTs)
			if err != nil {
				log.Printf("dedup check failed: %v", err)
				continue
			}
			if !isNew {
				log.Printf("duplicate event %s, skipping", event.ID)
				continue
			}

			// Validate the zap receipt
			validatedZap, err := zaps.ValidateZapReceipt(event, cfg.Lightning.LnurlPubkeyHex)
			if err != nil {
				if errors.Is(err, zaps.ErrUnauthorizedZapProvider) {
					log.Printf("zap from unauthorized provider: %v", err)
				} else {
					log.Printf("invalid zap receipt: %v", err)
				}
				_ = database.SetHighWaterMark(eventTs)
				continue
			}

			log.Printf("valid zap: %d sats from %s", validatedZap.AmountSats, validatedZap.SenderNpub)

			// Process the zap
			processResult, err := zaps.ProcessZap(ctx, database, validatedZap)
			if err != nil {
				if errors.Is(err, zaps.ErrDuplicateZap) {
					log.Printf("duplicate zap event %s, ignoring", validatedZap.ZapEventID)
				} else {
					log.Printf("failed to process zap: %v", err)
					if err := processorFSM.Event(ctx, fsm.ProcessorEventError); err != nil {
						log.Printf("FSM error on zap process error: %v", err)
					}
				}
				processorFSM.Reset()
				_ = database.SetHighWaterMark(eventTs)
				continue
			}

			// Transition FSM to sending response state
			if err := processorFSM.Event(ctx, fsm.ProcessorEventResponseSent); err != nil {
				log.Printf("FSM error on response sent (zap): %v", err)
				processorFSM.Reset()
				_ = database.SetHighWaterMark(eventTs)
				continue
			}

			log.Printf("zap processed: %s", processResult.Message)

			// Send DM confirmation to zapper
			_, senderPubkeyHex, err := nip19.Decode(validatedZap.SenderNpub)
			if err != nil {
				log.Printf("failed to decode sender npub: %v", err)
			} else {
				sendResponse(ctx, kr, relayMgr, cfg.Nostr.BotSecretHex, cfg.Nostr.BotPubkeyHex,
					senderPubkeyHex.(string), processResult.Message, dm.ProtocolNIP04)
			}

			// Notify admins of payment received
			adminMsg := fmt.Sprintf("💰 Payment received from %s:\n%s", validatedZap.SenderNpub, processResult.Message)
			notifyAdmins(ctx, kr, relayMgr, cfg, adminMsg)

			// Reset FSM to idle after zap processing completes
			processorFSM.Reset()
			_ = database.SetHighWaterMark(eventTs)
		}
	}
}

// sendResponse wraps a message in the appropriate protocol (NIP-04 or NIP-17) and publishes it to relays.
func sendResponse(ctx context.Context, kr gonostr.Keyer, relayMgr *nostr.RelayManager, botSecretHex, botPubkeyHex, recipientPubkeyHex, message string, protocol dm.DMProtocol) {
	var wrapped *gonostr.Event
	var err error

	switch protocol {
	case dm.ProtocolNIP04:
		wrapped, err = dm.WrapLegacyResponse(ctx, kr, botSecretHex, botPubkeyHex, recipientPubkeyHex, message)
	case dm.ProtocolNIP17:
		wrapped, err = dm.WrapResponse(ctx, kr, botPubkeyHex, recipientPubkeyHex, message)
	default:
		// Default to NIP-17 for safety
		wrapped, err = dm.WrapResponse(ctx, kr, botPubkeyHex, recipientPubkeyHex, message)
	}

	if err != nil {
		log.Printf("failed to wrap response: %v", err)
		return
	}

	if err := relayMgr.Publish(ctx, wrapped); err != nil {
		log.Printf("failed to publish response: %v", err)
		return
	}

	// Convert hex to npub for display
	recipientNpub, _ := nip19.EncodePublicKey(recipientPubkeyHex)
	log.Printf("sent response to %s", recipientNpub)
}

// broadcastPrefix is the command prefix for admin broadcast messages.
const broadcastPrefix = "message customers:"

// parseBroadcast checks if content is a broadcast command and extracts the message.
func parseBroadcast(content string) (message string, isBroadcast bool) {
	content = stripMarkdownComments(content)
	content = strings.TrimSpace(content)
	lower := strings.ToLower(content)
	if !strings.HasPrefix(lower, broadcastPrefix) {
		return "", false
	}
	message = strings.TrimSpace(content[len(broadcastPrefix):])
	return message, true
}

// stripMarkdownComments removes markdown reference-style link definitions
// that some Nostr clients prepend to messages, e.g. "[//]: # (nip18)"
func stripMarkdownComments(content string) string {
	lines := strings.Split(content, "\n")
	var result []string
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "[//]:") {
			continue
		}
		result = append(result, line)
	}
	return strings.Join(result, "\n")
}

// broadcastToCustomers sends a DM to all registered customers.
func broadcastToCustomers(ctx context.Context, kr gonostr.Keyer, relayMgr *nostr.RelayManager,
	cfg *config.Config, database *db.DB, message string) (sent int, failed int) {

	customers, err := database.ListCustomers(ctx)
	if err != nil {
		log.Printf("failed to list customers for broadcast: %v", err)
		return 0, 0
	}

	for _, customer := range customers {
		_, pubkeyHex, err := nip19.Decode(customer.Npub)
		if err != nil {
			log.Printf("failed to decode customer npub %s: %v", customer.Npub, err)
			failed++
			continue
		}
		sendResponse(ctx, kr, relayMgr, cfg.Nostr.BotSecretHex, cfg.Nostr.BotPubkeyHex,
			pubkeyHex.(string), message, dm.ProtocolNIP04)
		sent++
	}
	return sent, failed
}

// notifyAdmins sends a DM to all configured admins.
func notifyAdmins(ctx context.Context, kr gonostr.Keyer, relayMgr *nostr.RelayManager, cfg *config.Config, message string) {
	for _, adminNpub := range cfg.Admins {
		_, adminPubkeyHex, err := nip19.Decode(adminNpub)
		if err != nil {
			log.Printf("failed to decode admin npub %s: %v", adminNpub, err)
			continue
		}
		sendResponse(ctx, kr, relayMgr, cfg.Nostr.BotSecretHex, cfg.Nostr.BotPubkeyHex,
			adminPubkeyHex.(string), message, dm.ProtocolNIP04)
	}
}

// checkInventoryNotifications checks for triggered notifications and sends DMs.
// Called after commands that may increase inventory (inventory add/set, cancel).
func checkInventoryNotifications(ctx context.Context, kr gonostr.Keyer, relayMgr *nostr.RelayManager,
	cfg *config.Config, database *db.DB) {

	available, err := database.GetInventory(ctx)
	if err != nil {
		log.Printf("failed to get inventory for notifications: %v", err)
		return
	}

	if available == 0 {
		return
	}

	notifications, err := database.GetTriggeredNotifications(ctx, available)
	if err != nil {
		log.Printf("failed to get triggered notifications: %v", err)
		return
	}

	for _, n := range notifications {
		_, pubkeyHex, err := nip19.Decode(n.CustomerNpub)
		if err != nil {
			log.Printf("failed to decode customer npub %s: %v", n.CustomerNpub, err)
			continue
		}

		msg := fmt.Sprintf("🥚 Inventory alert: %d eggs are now available!", available)
		sendResponse(ctx, kr, relayMgr, cfg.Nostr.BotSecretHex, cfg.Nostr.BotPubkeyHex,
			pubkeyHex.(string), msg, dm.ProtocolNIP04)

		if err := database.DeleteInventoryNotificationByID(ctx, n.ID); err != nil {
			log.Printf("failed to delete notification %d: %v", n.ID, err)
		} else {
			log.Printf("sent inventory notification to %s (threshold: %d)", n.CustomerNpub, n.ThresholdEggs)
		}
	}
}
