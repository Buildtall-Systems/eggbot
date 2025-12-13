package cli

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/buildtall-systems/eggbot/internal/config"
	"github.com/buildtall-systems/eggbot/internal/db"
	"github.com/buildtall-systems/eggbot/internal/nostr"
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
			// TODO: Phase 3 - Unwrap and process DM

		case event := <-relayMgr.ZapEvents():
			if event == nil {
				continue
			}
			log.Printf("received zap event: %s (kind:%d)", event.ID[:8], event.Kind)
			// TODO: Phase 5 - Process zap payment
		}
	}
}
