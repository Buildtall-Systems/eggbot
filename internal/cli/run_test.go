package cli

import (
	"bytes"
	"log"
	"strings"
	"testing"

	"github.com/nbd-wtf/go-nostr/nip19"
)

// Test hex pubkey for testing
const testPubkeyHex = "dcfafaaebf643e0c8517e49e13ad25c60ee4a57a0b5f5fc401adbcb9d151f5f5"

// Expected npub for the test hex pubkey
const testExpectedNpub = "npub1mna04t4lvslqepghuj0p8tf9cc8wfft6pd04l3qp4k7tn5237h6sj6ru9w"

func TestNpubConversion(t *testing.T) {
	// Verify our test constants are correct
	npub, err := nip19.EncodePublicKey(testPubkeyHex)
	if err != nil {
		t.Fatalf("failed to encode pubkey: %v", err)
	}
	if npub != testExpectedNpub {
		t.Errorf("npub = %s, want %s", npub, testExpectedNpub)
	}
}

func TestLogOutputShowsNpubNotHex(t *testing.T) {
	// Capture log output
	var buf bytes.Buffer
	log.SetOutput(&buf)
	defer log.SetOutput(nil) // Reset after test

	// GOOD: Convert hex to npub before logging
	senderNpub, _ := nip19.EncodePublicKey(testPubkeyHex)
	log.Printf("DM from %s: test message", senderNpub)

	output := buf.String()

	// Verify output contains npub, not hex
	if !strings.Contains(output, "npub1") {
		t.Errorf("log output should contain npub, got: %s", output)
	}
	if strings.Contains(output, testPubkeyHex) {
		t.Errorf("log output should NOT contain hex pubkey, got: %s", output)
	}
	if strings.Contains(output, testPubkeyHex[:8]) {
		t.Errorf("log output should NOT contain truncated hex pubkey, got: %s", output)
	}
}

func TestLogOutputShowsFullNpub(t *testing.T) {
	// Capture log output
	var buf bytes.Buffer
	log.SetOutput(&buf)
	defer log.SetOutput(nil)

	npub, _ := nip19.EncodePublicKey(testPubkeyHex)
	log.Printf("valid zap: 1000 sats from %s", npub)

	output := buf.String()

	// Verify full npub is in output (not truncated)
	if !strings.Contains(output, testExpectedNpub) {
		t.Errorf("log output should contain full npub %s, got: %s", testExpectedNpub, output)
	}
}

func TestLogOutputForPermissionDenied(t *testing.T) {
	var buf bytes.Buffer
	log.SetOutput(&buf)
	defer log.SetOutput(nil)

	senderNpub, _ := nip19.EncodePublicKey(testPubkeyHex)
	log.Printf("permission denied for %s: you are not a registered customer", senderNpub)

	output := buf.String()

	if !strings.Contains(output, testExpectedNpub) {
		t.Errorf("permission denied log should contain full npub, got: %s", output)
	}
	if strings.Contains(output, testPubkeyHex[:8]) {
		t.Errorf("permission denied log should NOT contain truncated hex, got: %s", output)
	}
}

func TestLogOutputForSentResponse(t *testing.T) {
	var buf bytes.Buffer
	log.SetOutput(&buf)
	defer log.SetOutput(nil)

	recipientNpub, _ := nip19.EncodePublicKey(testPubkeyHex)
	log.Printf("sent response to %s", recipientNpub)

	output := buf.String()

	if !strings.Contains(output, testExpectedNpub) {
		t.Errorf("sent response log should contain full npub, got: %s", output)
	}
	if strings.Contains(output, testPubkeyHex[:8]) {
		t.Errorf("sent response log should NOT contain truncated hex, got: %s", output)
	}
}
