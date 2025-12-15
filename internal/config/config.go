package config

import (
	"fmt"
	"os"

	"github.com/nbd-wtf/go-nostr"
	"github.com/nbd-wtf/go-nostr/nip19"
	"github.com/spf13/viper"
)

// Config holds all application configuration.
type Config struct {
	Verbose   bool
	Database  DatabaseConfig
	Nostr     NostrConfig
	Lightning LightningConfig
	Pricing   PricingConfig
	Admins    []string // npubs of admin users
}

// DatabaseConfig holds database settings.
type DatabaseConfig struct {
	Path string
}

// NostrConfig holds Nostr-related settings.
type NostrConfig struct {
	Relays        []string
	BotNpub       string // Bot's public key in npub format (from config)
	BotSecretHex  string // Bot's secret key in hex (derived from EGGBOT_NSEC env)
	BotPubkeyHex  string // Bot's public key in hex (derived from secret)
}

// LightningConfig holds Lightning payment settings.
type LightningConfig struct {
	LnurlNpub        string // LNURL provider's npub (from config)
	LnurlPubkeyHex   string // Derived hex pubkey for zap validation
	LightningAddress string // Lightning address for payments (e.g., user@getalby.com)
}

// PricingConfig holds egg pricing settings.
type PricingConfig struct {
	SatsPerHalfDozen int // Price for 6 eggs in sats
}

// Load reads configuration from Viper and returns a Config struct.
// Does not load secrets - use LoadWithSecrets for full runtime config.
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
		Lightning: LightningConfig{
			LnurlNpub:        viper.GetString("lightning.lnurl_npub"),
			LightningAddress: viper.GetString("lightning.address"),
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

// LoadWithSecrets loads config and derives bot keypair from EGGBOT_NSEC env var.
// Returns error if EGGBOT_NSEC is not set or invalid.
func LoadWithSecrets() (*Config, error) {
	cfg, err := Load()
	if err != nil {
		return nil, err
	}

	nsec := os.Getenv("EGGBOT_NSEC")
	if nsec == "" {
		return nil, fmt.Errorf("EGGBOT_NSEC environment variable is required")
	}

	// Decode nsec to get hex secret key
	prefix, value, err := nip19.Decode(nsec)
	if err != nil {
		return nil, fmt.Errorf("invalid EGGBOT_NSEC: %w", err)
	}
	if prefix != "nsec" {
		return nil, fmt.Errorf("EGGBOT_NSEC must be an nsec, got %s", prefix)
	}

	secretHex, ok := value.(string)
	if !ok {
		return nil, fmt.Errorf("failed to decode nsec value")
	}

	// Derive public key from secret
	pubkeyHex, err := nostr.GetPublicKey(secretHex)
	if err != nil {
		return nil, fmt.Errorf("deriving public key: %w", err)
	}

	cfg.Nostr.BotSecretHex = secretHex
	cfg.Nostr.BotPubkeyHex = pubkeyHex

	// Derive BotNpub from pubkeyHex (for display purposes)
	derivedNpub, err := nip19.EncodePublicKey(pubkeyHex)
	if err != nil {
		return nil, fmt.Errorf("encoding bot npub: %w", err)
	}

	// Verify derived pubkey matches config if specified, then always set BotNpub
	if cfg.Nostr.BotNpub != "" {
		configPrefix, configValue, err := nip19.Decode(cfg.Nostr.BotNpub)
		if err != nil {
			return nil, fmt.Errorf("invalid nostr.bot_npub in config: %w", err)
		}
		if configPrefix != "npub" {
			return nil, fmt.Errorf("nostr.bot_npub must be an npub, got %s", configPrefix)
		}
		configPubkeyHex, ok := configValue.(string)
		if !ok {
			return nil, fmt.Errorf("failed to decode npub value")
		}
		if configPubkeyHex != pubkeyHex {
			return nil, fmt.Errorf("EGGBOT_NSEC does not match nostr.bot_npub in config")
		}
	} else {
		cfg.Nostr.BotNpub = derivedNpub
	}

	// Derive LNURL provider pubkey hex if specified
	if cfg.Lightning.LnurlNpub != "" {
		lnPrefix, lnValue, err := nip19.Decode(cfg.Lightning.LnurlNpub)
		if err != nil {
			return nil, fmt.Errorf("invalid lightning.lnurl_npub: %w", err)
		}
		if lnPrefix != "npub" {
			return nil, fmt.Errorf("lightning.lnurl_npub must be an npub, got %s", lnPrefix)
		}
		lnPubkeyHex, ok := lnValue.(string)
		if !ok {
			return nil, fmt.Errorf("failed to decode lnurl_npub value")
		}
		cfg.Lightning.LnurlPubkeyHex = lnPubkeyHex
	}

	return cfg, nil
}
