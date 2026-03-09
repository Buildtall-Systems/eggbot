package lightning

import (
	"encoding/hex"
	"fmt"
	"net/url"
	"strings"
)

type ConnectionParams struct {
	WalletPubkey string
	RelayURL     string
	AppKey       string
}

func ParseConnectionString(connStr string) (*ConnectionParams, error) {
	if !strings.HasPrefix(connStr, "nostr+walletconnect://") {
		return nil, fmt.Errorf("invalid scheme: must start with nostr+walletconnect://")
	}

	raw := strings.TrimPrefix(connStr, "nostr+walletconnect://")

	parts := strings.SplitN(raw, "?", 2)
	if len(parts) != 2 {
		return nil, fmt.Errorf("missing query parameters")
	}

	walletPubkey := parts[0]
	if err := validateHex64(walletPubkey, "wallet pubkey"); err != nil {
		return nil, err
	}

	query, err := url.ParseQuery(parts[1])
	if err != nil {
		return nil, fmt.Errorf("invalid query string: %w", err)
	}

	relayURL := query.Get("relay")
	if relayURL == "" {
		return nil, fmt.Errorf("missing required parameter: relay")
	}
	if !strings.HasPrefix(relayURL, "wss://") && !strings.HasPrefix(relayURL, "ws://") {
		return nil, fmt.Errorf("relay URL must start with wss:// or ws://")
	}

	secret := query.Get("secret")
	if secret == "" {
		return nil, fmt.Errorf("missing required parameter: secret")
	}
	if err := validateHex64(secret, "secret"); err != nil {
		return nil, err
	}

	return &ConnectionParams{
		WalletPubkey: walletPubkey,
		RelayURL:     relayURL,
		AppKey:       secret,
	}, nil
}

func validateHex64(s, name string) error {
	if len(s) != 64 {
		return fmt.Errorf("%s must be 64 hex characters, got %d", name, len(s))
	}
	if _, err := hex.DecodeString(s); err != nil {
		return fmt.Errorf("%s is not valid hex: %w", name, err)
	}
	return nil
}
