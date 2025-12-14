package dm

import (
	"context"
	"testing"

	"github.com/nbd-wtf/go-nostr"
	"github.com/nbd-wtf/go-nostr/keyer"
	"github.com/nbd-wtf/go-nostr/nip59"
)

// Test keypairs (generated with nak):
// Bot (Customer from progress file):
//   Secret: 234702910939c3394838131938e8da0dcfec369df3e51990263eae626aa73f87
//   Pubkey: 1eca03bebec0590b918861b4431d57ff574702fa8cb015ccd566b509e9480c42
//
// Recipient (Unknown from progress file):
//   Secret: d067b66a004de257ff3f467e754d22bb2b64a9a59c669e8224d8c624b7decb4f
//   Pubkey: dcfafaaebf643e0c8517e49e13ad25c60ee4a57a0b5f5fc401adbcb9d151f5f5

const botSecretHex = "234702910939c3394838131938e8da0dcfec369df3e51990263eae626aa73f87"
const botPubkeyHex = "1eca03bebec0590b918861b4431d57ff574702fa8cb015ccd566b509e9480c42"
const recipientSecretHex = "d067b66a004de257ff3f467e754d22bb2b64a9a59c669e8224d8c624b7decb4f"
const recipientPubkeyHex = "dcfafaaebf643e0c8517e49e13ad25c60ee4a57a0b5f5fc401adbcb9d151f5f5"

func TestWrapResponse(t *testing.T) {
	ctx := context.Background()

	// Create bot keyer
	kr, err := keyer.NewPlainKeySigner(botSecretHex)
	if err != nil {
		t.Fatalf("creating keyer: %v", err)
	}

	message := "Hello, this is a test response!"

	// Wrap the response
	wrapped, err := WrapResponse(ctx, kr, botPubkeyHex, recipientPubkeyHex, message)
	if err != nil {
		t.Fatalf("WrapResponse() error = %v", err)
	}

	// Verify it's a gift wrap (kind 1059)
	if wrapped.Kind != nostr.KindGiftWrap {
		t.Errorf("wrapped.Kind = %d, want %d (KindGiftWrap)", wrapped.Kind, nostr.KindGiftWrap)
	}

	// Verify it has a p tag for the recipient
	pTag := wrapped.Tags.Find("p")
	if len(pTag) < 2 {
		t.Error("wrapped event missing p tag")
	} else if pTag[1] != recipientPubkeyHex {
		t.Errorf("p tag = %s, want %s", pTag[1], recipientPubkeyHex)
	}

	// Verify signature is valid
	ok, err := wrapped.CheckSignature()
	if err != nil || !ok {
		t.Errorf("wrapped event has invalid signature: %v", err)
	}
}

func TestWrapResponse_CanBeUnwrapped(t *testing.T) {
	ctx := context.Background()

	// Create bot keyer
	botKr, err := keyer.NewPlainKeySigner(botSecretHex)
	if err != nil {
		t.Fatalf("creating bot keyer: %v", err)
	}

	// Create recipient keyer
	recipientKr, err := keyer.NewPlainKeySigner(recipientSecretHex)
	if err != nil {
		t.Fatalf("creating recipient keyer: %v", err)
	}

	message := "This message should be decryptable by the recipient"

	// Wrap the response
	wrapped, err := WrapResponse(ctx, botKr, botPubkeyHex, recipientPubkeyHex, message)
	if err != nil {
		t.Fatalf("WrapResponse() error = %v", err)
	}

	// Recipient unwraps the gift
	rumor, err := nip59.GiftUnwrap(*wrapped, func(pubkey, ciphertext string) (string, error) {
		return recipientKr.Decrypt(ctx, ciphertext, pubkey)
	})
	if err != nil {
		t.Fatalf("GiftUnwrap() error = %v", err)
	}

	// Verify the unwrapped message
	if rumor.Kind != nostr.KindDirectMessage {
		t.Errorf("rumor.Kind = %d, want %d (KindDirectMessage)", rumor.Kind, nostr.KindDirectMessage)
	}

	if rumor.Content != message {
		t.Errorf("rumor.Content = %s, want %s", rumor.Content, message)
	}

	// Verify sender is the bot
	if rumor.PubKey != botPubkeyHex {
		t.Errorf("rumor.PubKey = %s, want %s", rumor.PubKey, botPubkeyHex)
	}

	// Verify p tag points to recipient
	pTag := rumor.Tags.Find("p")
	if len(pTag) < 2 {
		t.Error("rumor missing p tag")
	} else if pTag[1] != recipientPubkeyHex {
		t.Errorf("p tag = %s, want %s", pTag[1], recipientPubkeyHex)
	}
}

func TestWrapResponse_DifferentMessages(t *testing.T) {
	ctx := context.Background()

	botKr, err := keyer.NewPlainKeySigner(botSecretHex)
	if err != nil {
		t.Fatalf("creating keyer: %v", err)
	}

	messages := []string{
		"Short",
		"A longer message with some content",
		"Message with special chars: !@#$%^&*()",
		"Multi\nline\nmessage",
		"Unicode: 日本語 🎉 émoji",
	}

	recipientKr, err := keyer.NewPlainKeySigner(recipientSecretHex)
	if err != nil {
		t.Fatalf("creating recipient keyer: %v", err)
	}

	for _, msg := range messages {
		t.Run(msg[:min(len(msg), 20)], func(t *testing.T) {
			wrapped, err := WrapResponse(ctx, botKr, botPubkeyHex, recipientPubkeyHex, msg)
			if err != nil {
				t.Fatalf("WrapResponse() error = %v", err)
			}

			rumor, err := nip59.GiftUnwrap(*wrapped, func(pubkey, ciphertext string) (string, error) {
				return recipientKr.Decrypt(ctx, ciphertext, pubkey)
			})
			if err != nil {
				t.Fatalf("GiftUnwrap() error = %v", err)
			}

			if rumor.Content != msg {
				t.Errorf("content = %q, want %q", rumor.Content, msg)
			}
		})
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
