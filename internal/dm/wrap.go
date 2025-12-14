package dm

import (
	"context"
	"fmt"

	"github.com/nbd-wtf/go-nostr"
	"github.com/nbd-wtf/go-nostr/nip59"
)

// WrapResponse creates a NIP-17 gift-wrapped DM response.
// recipientPubkeyHex is the hex pubkey of the recipient.
// Returns a ready-to-publish kind:1059 gift-wrapped event.
func WrapResponse(ctx context.Context, kr nostr.Keyer, botPubkeyHex string, recipientPubkeyHex string, message string) (*nostr.Event, error) {
	// Create the rumor (kind:14 direct message)
	rumor := nostr.Event{
		PubKey:    botPubkeyHex,
		CreatedAt: nostr.Now(),
		Kind:      nostr.KindDirectMessage, // kind:14
		Tags: nostr.Tags{
			nostr.Tag{"p", recipientPubkeyHex},
		},
		Content: message,
	}

	// Gift wrap the rumor using NIP-59
	// This creates: rumor -> seal (kind:13) -> gift wrap (kind:1059)
	giftWrap, err := nip59.GiftWrap(
		rumor,
		recipientPubkeyHex,
		// Encrypt function - encrypts plaintext for recipient
		func(plaintext string) (string, error) {
			return kr.Encrypt(ctx, plaintext, recipientPubkeyHex)
		},
		// Sign function - signs the seal event
		func(event *nostr.Event) error {
			return kr.SignEvent(ctx, event)
		},
		// No modification function needed
		nil,
	)
	if err != nil {
		return nil, fmt.Errorf("gift wrapping response: %w", err)
	}

	return &giftWrap, nil
}
