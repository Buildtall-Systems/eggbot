package lightning

import (
	"net/url"
	"strings"
	"testing"
)

func TestParseNWCConnectionString(t *testing.T) {
	validPubkey := "b889ff5b1513b641e2a139f661a661364979c5beee91842f8f0ef42ab558e9d4"
	validSecret := "71a8c14c1407c113601079c4302dab36460f0ccd0ad506f1f2dc73b5100571a7"
	validRelay := "wss://relay.getalby.com/v1"

	validConnStr := "nostr+walletconnect://" + validPubkey +
		"?relay=" + url.QueryEscape(validRelay) +
		"&secret=" + validSecret

	tests := []struct {
		name    string
		input   string
		check   func(t *testing.T, p *NWCParams)
		wantErr string
	}{
		{
			name:  "valid connection string",
			input: validConnStr,
			check: func(t *testing.T, p *NWCParams) {
				if p.WalletPubkey != validPubkey {
					t.Errorf("WalletPubkey = %s, want %s", p.WalletPubkey, validPubkey)
				}
				if p.RelayURL != validRelay {
					t.Errorf("RelayURL = %s, want %s", p.RelayURL, validRelay)
				}
				if p.AppKey != validSecret {
					t.Errorf("Secret = %s, want %s", p.AppKey, validSecret)
				}
			},
		},
		{
			name:  "valid with unencoded relay",
			input: "nostr+walletconnect://" + validPubkey + "?relay=wss://relay.example.com&secret=" + validSecret,
			check: func(t *testing.T, p *NWCParams) {
				if p.RelayURL != "wss://relay.example.com" {
					t.Errorf("RelayURL = %s, want wss://relay.example.com", p.RelayURL)
				}
			},
		},
		{
			name:    "wrong scheme",
			input:   "https://" + validPubkey + "?relay=" + validRelay + "&secret=" + validSecret,
			wantErr: "invalid scheme",
		},
		{
			name:    "missing query parameters",
			input:   "nostr+walletconnect://" + validPubkey,
			wantErr: "missing query parameters",
		},
		{
			name:    "missing relay",
			input:   "nostr+walletconnect://" + validPubkey + "?secret=" + validSecret,
			wantErr: "missing required parameter: relay",
		},
		{
			name:    "missing secret",
			input:   "nostr+walletconnect://" + validPubkey + "?relay=" + validRelay,
			wantErr: "missing required parameter: secret",
		},
		{
			name:    "short pubkey",
			input:   "nostr+walletconnect://abcd?relay=" + validRelay + "&secret=" + validSecret,
			wantErr: "wallet pubkey must be 64 hex characters",
		},
		{
			name:    "non-hex pubkey",
			input:   "nostr+walletconnect://" + strings.Repeat("zz", 32) + "?relay=" + validRelay + "&secret=" + validSecret,
			wantErr: "wallet pubkey is not valid hex",
		},
		{
			name:    "short secret",
			input:   "nostr+walletconnect://" + validPubkey + "?relay=" + validRelay + "&secret=abcd",
			wantErr: "secret must be 64 hex characters",
		},
		{
			name:    "non-hex secret",
			input:   "nostr+walletconnect://" + validPubkey + "?relay=" + validRelay + "&secret=" + strings.Repeat("zz", 32),
			wantErr: "secret is not valid hex",
		},
		{
			name:    "http relay",
			input:   "nostr+walletconnect://" + validPubkey + "?relay=https://example.com&secret=" + validSecret,
			wantErr: "relay URL must start with wss:// or ws://",
		},
		{
			name:  "ws relay accepted",
			input: "nostr+walletconnect://" + validPubkey + "?relay=ws://localhost:7777&secret=" + validSecret,
			check: func(t *testing.T, p *NWCParams) {
				if p.RelayURL != "ws://localhost:7777" {
					t.Errorf("RelayURL = %s, want ws://localhost:7777", p.RelayURL)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseNWCConnectionString(tt.input)
			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tt.wantErr)
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("error = %q, want it to contain %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tt.check != nil {
				tt.check(t, got)
			}
		})
	}
}
