package cli

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/buildtall-systems/eggbot/internal/commands"
	"github.com/buildtall-systems/eggbot/internal/config"
	"github.com/buildtall-systems/eggbot/internal/db"
	"github.com/buildtall-systems/eggbot/internal/nostr"
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
			cmd := commands.Parse(rumor.Content)
			if cmd == nil {
				log.Printf("empty message, ignoring")
				continue
			}

			if !cmd.IsValid() {
				log.Printf("unknown command: %s", cmd.Name)
				// TODO: Send help response
				continue
			}

			// Check permissions
			if err := commands.CanExecute(ctx, database.DB, cmd, rumor.PubKey, cfg.Admins); err != nil {
				log.Printf("permission denied for %s: %v", rumor.PubKey[:8], err)
				// TODO: Send error response
				continue
			}

			log.Printf("executing command: %s %v", cmd.Name, cmd.Args)

			// Execute the command
			execCfg := commands.ExecuteConfig{
				SatsPerHalfDozen: cfg.Pricing.SatsPerHalfDozen,
				Admins:           cfg.Admins,
			}
			result := commands.Execute(ctx, database, cmd, rumor.PubKey, execCfg)

			if result.Error != nil {
				log.Printf("command error: %v", result.Error)
				// TODO: Phase 5 - Send error response via DM
			} else {
				log.Printf("command result: %s", result.Message)
				// TODO: Phase 5 - Send response via DM
			}

		case event := <-relayMgr.ZapEvents():
			if event == nil {
				continue
			}
			log.Printf("received zap event: %s (kind:%d)", event.ID[:8], event.Kind)
			// TODO: Phase 5 - Process zap payment
		}
	}
}
