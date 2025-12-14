package cli

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/buildtall-systems/eggbot/internal/commands"
	"github.com/buildtall-systems/eggbot/internal/config"
	"github.com/buildtall-systems/eggbot/internal/db"
	"github.com/buildtall-systems/eggbot/internal/dm"
	"github.com/buildtall-systems/eggbot/internal/nostr"
	"github.com/buildtall-systems/eggbot/internal/zaps"
	gonostr "github.com/nbd-wtf/go-nostr"
	"github.com/nbd-wtf/go-nostr/keyer"
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
	log.Printf("bot pubkey: %s", cfg.Nostr.BotPubkeyHex)
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

	// Create and connect relay manager
	relayMgr := nostr.NewRelayManager(cfg.Nostr.Relays, cfg.Nostr.BotPubkeyHex)
	if err := relayMgr.Connect(ctx); err != nil {
		return fmt.Errorf("connecting to relays: %w", err)
	}
	defer relayMgr.Close()

	log.Printf("eggbot running, waiting for events...")

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
			log.Printf("received DM event: %s (kind:%d)", event.ID[:8], event.Kind)

			// Unwrap the gift-wrapped DM to get the rumor (actual message)
			rumor, err := nip59.GiftUnwrap(*event, func(pubkey, ciphertext string) (string, error) {
				return kr.Decrypt(ctx, ciphertext, pubkey)
			})
			if err != nil {
				log.Printf("failed to unwrap DM: %v", err)
				continue
			}

			log.Printf("DM from %s: %s", rumor.PubKey[:8], rumor.Content)

			// Parse command from message
			parsedCmd := commands.Parse(rumor.Content)
			if parsedCmd == nil {
				log.Printf("empty message, ignoring")
				continue
			}

			if !parsedCmd.IsValid() {
				log.Printf("unknown command: %s", parsedCmd.Name)
				sendResponse(ctx, kr, relayMgr, cfg.Nostr.BotPubkeyHex, rumor.PubKey,
					fmt.Sprintf("Unknown command: %s. Send 'help' for available commands.", parsedCmd.Name))
				continue
			}

			// Check permissions
			if err := commands.CanExecute(ctx, database.DB, parsedCmd, rumor.PubKey, cfg.Admins); err != nil {
				log.Printf("permission denied for %s: %v", rumor.PubKey[:8], err)
				sendResponse(ctx, kr, relayMgr, cfg.Nostr.BotPubkeyHex, rumor.PubKey,
					fmt.Sprintf("Permission denied: %v", err))
				continue
			}

			log.Printf("executing command: %s %v", parsedCmd.Name, parsedCmd.Args)

			// Execute the command
			execCfg := commands.ExecuteConfig{
				SatsPerHalfDozen: cfg.Pricing.SatsPerHalfDozen,
				Admins:           cfg.Admins,
			}
			result := commands.Execute(ctx, database, parsedCmd, rumor.PubKey, execCfg)

			// Send response via DM
			var responseMsg string
			if result.Error != nil {
				log.Printf("command error: %v", result.Error)
				responseMsg = fmt.Sprintf("Error: %v", result.Error)
			} else {
				log.Printf("command result: %s", result.Message)
				responseMsg = result.Message
			}
			sendResponse(ctx, kr, relayMgr, cfg.Nostr.BotPubkeyHex, rumor.PubKey, responseMsg)

		case event := <-relayMgr.ZapEvents():
			if event == nil {
				continue
			}
			log.Printf("received zap event: %s (kind:%d)", event.ID[:8], event.Kind)

			// Validate the zap receipt
			validatedZap, err := zaps.ValidateZapReceipt(event, cfg.Lightning.LnurlPubkeyHex)
			if err != nil {
				if errors.Is(err, zaps.ErrUnauthorizedZapProvider) {
					log.Printf("zap from unauthorized provider: %v", err)
				} else {
					log.Printf("invalid zap receipt: %v", err)
				}
				continue
			}

			log.Printf("valid zap: %d sats from %s", validatedZap.AmountSats, validatedZap.SenderNpub[:16])

			// Process the zap
			processResult, err := zaps.ProcessZap(ctx, database, validatedZap)
			if err != nil {
				if errors.Is(err, zaps.ErrDuplicateZap) {
					log.Printf("duplicate zap event %s, ignoring", validatedZap.ZapEventID[:8])
				} else {
					log.Printf("failed to process zap: %v", err)
				}
				continue
			}

			log.Printf("zap processed: %s", processResult.Message)
		}
	}
}

// sendResponse wraps a message in NIP-17 gift wrap and publishes it to relays.
func sendResponse(ctx context.Context, kr gonostr.Keyer, relayMgr *nostr.RelayManager, botPubkeyHex, recipientPubkeyHex, message string) {
	wrapped, err := dm.WrapResponse(ctx, kr, botPubkeyHex, recipientPubkeyHex, message)
	if err != nil {
		log.Printf("failed to wrap response: %v", err)
		return
	}

	if err := relayMgr.Publish(ctx, wrapped); err != nil {
		log.Printf("failed to publish response: %v", err)
		return
	}

	log.Printf("sent response to %s", recipientPubkeyHex[:8])
}
