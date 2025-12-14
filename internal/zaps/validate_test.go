package zaps

import (
	"encoding/json"
	"testing"

	"github.com/nbd-wtf/go-nostr"
	"github.com/nbd-wtf/go-nostr/nip19"
)

func TestExtractAmountFromBolt11(t *testing.T) {
	tests := []struct {
		name     string
		invoice  string
		wantMsat int64
		wantErr  bool
	}{
		{
			name:     "100 sats (100u)",
			invoice:  "lnbc100u1pnxyzabc",
			wantMsat: 10_000_000, // 100 * 100,000 (micro-BTC to msats)
			wantErr:  false,
		},
		{
			name:     "1 milli-BTC (1m) = 100,000 sats",
			invoice:  "lnbc1m1pnxyzabc",
			wantMsat: 100_000_000, // 1 mBTC = 100,000 sats = 100,000,000 msats
			wantErr:  false,
		},
		{
			name:     "21 sats (21000n)",
			invoice:  "lnbc21000n1pnxyzabc",
			wantMsat: 2_100_000, // 21000 * 100 msats
			wantErr:  false,
		},
		{
			name:     "10000 sats (10000u)",
			invoice:  "lnbc10000u1pnxyzabc",
			wantMsat: 1_000_000_000, // 10000 * 100,000 msats
			wantErr:  false,
		},
		{
			name:     "testnet invoice",
			invoice:  "lntb500u1pnxyzabc",
			wantMsat: 50_000_000, // 500 * 100,000
			wantErr:  false,
		},
		{
			name:     "regtest invoice",
			invoice:  "lnbcrt2500u1pnxyzabc",
			wantMsat: 250_000_000, // 2500 * 100,000
			wantErr:  false,
		},
		{
			name:    "invalid prefix",
			invoice: "invalid1pnxyzabc",
			wantErr: true,
		},
		{
			name:    "no separator (no 1)",
			invoice: "lnbc500upnxyzabc",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := extractAmountFromBolt11(tt.invoice)
			if (err != nil) != tt.wantErr {
				t.Errorf("extractAmountFromBolt11() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && got != tt.wantMsat {
				t.Errorf("extractAmountFromBolt11() = %d msats, want %d msats", got, tt.wantMsat)
			}
		})
	}
}

func TestValidateZapReceipt_InvalidKind(t *testing.T) {
	event := &nostr.Event{
		Kind: 1, // Not a zap receipt
	}

	_, err := ValidateZapReceipt(event, "")
	if err == nil {
		t.Error("expected error for wrong kind")
	}
}

func TestValidateZapReceipt_MissingDescription(t *testing.T) {
	// Create a minimal zap event without description
	event := &nostr.Event{
		Kind:    nostr.KindZap,
		Content: "",
		Tags:    nostr.Tags{},
	}

	// Sign it
	sk := "234702910939c3394838131938e8da0dcfec369df3e51990263eae626aa73f87" // test key
	_ = event.Sign(sk)

	_, err := ValidateZapReceipt(event, "")
	if err == nil {
		t.Error("expected error for missing description tag")
	}
}

func TestValidateZapReceipt_InvalidZapRequest(t *testing.T) {
	// Create zap event with invalid JSON in description
	event := &nostr.Event{
		Kind:    nostr.KindZap,
		Content: "",
		Tags: nostr.Tags{
			{"description", "not valid json"},
			{"bolt11", "lnbc100u1pjtest"},
		},
	}

	sk := "234702910939c3394838131938e8da0dcfec369df3e51990263eae626aa73f87"
	_ = event.Sign(sk)

	_, err := ValidateZapReceipt(event, "")
	if err == nil {
		t.Error("expected error for invalid zap request JSON")
	}
}

func TestValidateZapReceipt_WrongZapRequestKind(t *testing.T) {
	// Create a valid-looking but wrong kind zap request
	zapRequest := nostr.Event{
		Kind:    1, // Wrong kind, should be 9734
		PubKey:  "dcfafaaebf643e0c8517e49e13ad25c60ee4a57a0b5f5fc401adbcb9d151f5f5",
		Content: "",
		Tags:    nostr.Tags{},
	}
	zapRequestJSON, _ := json.Marshal(zapRequest)

	event := &nostr.Event{
		Kind:    nostr.KindZap,
		Content: "",
		Tags: nostr.Tags{
			{"description", string(zapRequestJSON)},
			{"bolt11", "lnbc100u1pjtest"},
		},
	}

	sk := "234702910939c3394838131938e8da0dcfec369df3e51990263eae626aa73f87"
	_ = event.Sign(sk)

	_, err := ValidateZapReceipt(event, "")
	if err == nil {
		t.Error("expected error for wrong zap request kind")
	}
}

func TestValidateZapReceipt_UnauthorizedProvider(t *testing.T) {
	// Create a valid zap request
	zapRequest := nostr.Event{
		Kind:      nostr.KindZapRequest,
		PubKey:    "dcfafaaebf643e0c8517e49e13ad25c60ee4a57a0b5f5fc401adbcb9d151f5f5",
		CreatedAt: nostr.Now(),
		Content:   "",
		Tags:      nostr.Tags{},
	}
	zapRequestJSON, _ := json.Marshal(zapRequest)

	event := &nostr.Event{
		Kind:      nostr.KindZap,
		CreatedAt: nostr.Now(),
		Content:   "",
		Tags: nostr.Tags{
			{"description", string(zapRequestJSON)},
			{"bolt11", "lnbc100u1pjtest"},
		},
	}

	// Sign with one key
	sk := "234702910939c3394838131938e8da0dcfec369df3e51990263eae626aa73f87"
	_ = event.Sign(sk)

	// But expect a different LNURL provider
	_, err := ValidateZapReceipt(event, "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	if err == nil {
		t.Error("expected error for unauthorized provider")
	}
}

func TestValidateZapReceipt_Valid(t *testing.T) {
	// Sender pubkey (the person zapping)
	senderPubkey := "dcfafaaebf643e0c8517e49e13ad25c60ee4a57a0b5f5fc401adbcb9d151f5f5"

	// Create a valid zap request (kind 9734)
	zapRequest := nostr.Event{
		Kind:      nostr.KindZapRequest,
		PubKey:    senderPubkey,
		CreatedAt: nostr.Now(),
		Content:   "",
		Tags: nostr.Tags{
			{"p", "80f10d3abbdda4db6f53ab6fa2c37db6fbc63cac32d23e87d140cfdd85c2c60f"}, // bot pubkey
		},
	}
	zapRequestJSON, _ := json.Marshal(zapRequest)

	// LNURL provider key
	providerSk := "234702910939c3394838131938e8da0dcfec369df3e51990263eae626aa73f87"
	providerPk, _ := nostr.GetPublicKey(providerSk)

	// Create zap receipt (kind 9735) signed by LNURL provider
	// 10u = 10 micro-BTC = 1000 sats (1 micro-BTC = 100 sats)
	event := &nostr.Event{
		Kind:      nostr.KindZap,
		CreatedAt: nostr.Now(),
		Content:   "",
		Tags: nostr.Tags{
			{"description", string(zapRequestJSON)},
			{"bolt11", "lnbc10u1pnxyzabcdef"}, // 10 micro-BTC = 1000 sats
			{"p", "80f10d3abbdda4db6f53ab6fa2c37db6fbc63cac32d23e87d140cfdd85c2c60f"},
		},
	}
	_ = event.Sign(providerSk)

	// Validate - provider check enabled
	result, err := ValidateZapReceipt(event, providerPk)
	if err != nil {
		t.Fatalf("ValidateZapReceipt() error = %v", err)
	}

	// Convert expected hex to npub for comparison
	expectedNpub, _ := nip19.EncodePublicKey(senderPubkey)
	if result.SenderNpub != expectedNpub {
		t.Errorf("SenderNpub = %s, want %s", result.SenderNpub, expectedNpub)
	}

	if result.AmountSats != 1000 {
		t.Errorf("AmountSats = %d, want 1000", result.AmountSats)
	}

	if result.ZapEventID != event.ID {
		t.Errorf("ZapEventID = %s, want %s", result.ZapEventID, event.ID)
	}

	// Validate - provider check disabled (empty string)
	result2, err := ValidateZapReceipt(event, "")
	if err != nil {
		t.Fatalf("ValidateZapReceipt() with empty provider error = %v", err)
	}

	if result2.AmountSats != 1000 {
		t.Errorf("AmountSats = %d, want 1000", result2.AmountSats)
	}
}
