package dm

import (
	"context"
	"fmt"

	"github.com/nbd-wtf/go-nostr"
	"github.com/nbd-wtf/go-nostr/nip04"
	"github.com/nbd-wtf/go-nostr/nip59"
)

// DMProtocol indicates which DM protocol to use for responses.
type DMProtocol int

const (
	ProtocolNIP04 DMProtocol = DMProtocol(nostr.KindEncryptedDirectMessage) // Legacy encrypted DM (kind:4)
	ProtocolNIP17 DMProtocol = DMProtocol(nostr.KindGiftWrap)               // Gift-wrapped DM (kind:1059)
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

// WrapLegacyResponse creates a NIP-04 encrypted DM response (kind:4).
// botSecretHex is the hex-encoded secret key of the bot.
// Returns a ready-to-publish kind:4 encrypted direct message event.
func WrapLegacyResponse(ctx context.Context, kr nostr.Keyer, botSecretHex, botPubkeyHex, recipientPubkeyHex, message string) (*nostr.Event, error) {
	// 1. Compute shared secret
	sharedSecret, err := nip04.ComputeSharedSecret(recipientPubkeyHex, botSecretHex)
	if err != nil {
		return nil, fmt.Errorf("computing shared secret: %w", err)
	}

	// 2. Encrypt message
	ciphertext, err := nip04.Encrypt(message, sharedSecret)
	if err != nil {
		return nil, fmt.Errorf("encrypting message: %w", err)
	}

	// 3. Build event
	event := &nostr.Event{
		PubKey:    botPubkeyHex,
		CreatedAt: nostr.Now(),
		Kind:      nostr.KindEncryptedDirectMessage,
		Tags:      nostr.Tags{nostr.Tag{"p", recipientPubkeyHex}},
		Content:   ciphertext,
	}

	// 4. Sign event
	if err := kr.SignEvent(ctx, event); err != nil {
		return nil, fmt.Errorf("signing event: %w", err)
	}

	return event, nil
}
