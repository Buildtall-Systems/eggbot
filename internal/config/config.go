package config

import (
	"github.com/spf13/viper"
)

// Config holds all application configuration.
type Config struct {
	Verbose  bool
	Database DatabaseConfig
	Nostr    NostrConfig
	Pricing  PricingConfig
	Admins   []string // npubs of admin users
}

// DatabaseConfig holds database settings.
type DatabaseConfig struct {
	Path string
}

// NostrConfig holds Nostr-related settings.
type NostrConfig struct {
	Relays  []string
	BotNpub string // Bot's public key in npub format
}

// PricingConfig holds egg pricing settings.
type PricingConfig struct {
	SatsPerHalfDozen int // Price for 6 eggs in sats
}

// Load reads configuration from Viper and returns a Config struct.
func Load() (*Config, error) {
	cfg := &Config{
		Verbose: viper.GetBool("verbose"),
		Database: DatabaseConfig{
			Path: viper.GetString("database.path"),
		},
		Nostr: NostrConfig{
			Relays:  viper.GetStringSlice("nostr.relays"),
			BotNpub: viper.GetString("nostr.bot_npub"),
		},
		Pricing: PricingConfig{
			SatsPerHalfDozen: viper.GetInt("pricing.sats_per_half_dozen"),
		},
		Admins: viper.GetStringSlice("admins"),
	}

	// Apply defaults
	if cfg.Database.Path == "" {
		cfg.Database.Path = "eggbot.db"
	}
	if len(cfg.Nostr.Relays) == 0 {
		cfg.Nostr.Relays = []string{"wss://relay.damus.io"}
	}
	if cfg.Pricing.SatsPerHalfDozen == 0 {
		cfg.Pricing.SatsPerHalfDozen = 3200
	}

	return cfg, nil
}
