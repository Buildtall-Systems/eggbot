package config

import (
	"github.com/spf13/viper"
)

type Config struct {
	Verbose  bool
	Database string
}

func Load() (*Config, error) {
	cfg := &Config{
		Verbose:  viper.GetBool("verbose"),
		Database: viper.GetString("database"),
	}

	if cfg.Database == "" {
		cfg.Database = "eggbot.db"
	}

	return cfg, nil
}
