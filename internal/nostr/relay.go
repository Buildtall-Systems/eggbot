package nostr

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/nbd-wtf/go-nostr"
)

// RelayManager handles connections to multiple Nostr relays and manages subscriptions.
type RelayManager struct {
	pool         *nostr.SimplePool
	relayURLs    []string
	botPubkeyHex string

	// Event channels for consumers
	dmEvents  chan *nostr.Event // kind:1059 gift-wrapped DMs
	zapEvents chan *nostr.Event // kind:9735 zap receipts

	cancel context.CancelFunc
}

// NewRelayManager creates a new relay manager for the given relay URLs.
func NewRelayManager(relayURLs []string, botPubkeyHex string) *RelayManager {
	return &RelayManager{
		relayURLs:    relayURLs,
		botPubkeyHex: botPubkeyHex,
		dmEvents:     make(chan *nostr.Event, 100),
		zapEvents:    make(chan *nostr.Event, 100),
	}
}

// Connect establishes connections to all configured relays and starts subscriptions.
// The since parameter filters events to only those with created_at > since.
// Pass 0 to receive all historical events.
func (rm *RelayManager) Connect(ctx context.Context, since int64) error {
	ctx, rm.cancel = context.WithCancel(ctx)

	// Create pool with penalty box for exponential backoff on failures
	rm.pool = nostr.NewSimplePool(ctx, nostr.WithPenaltyBox())

	// Subscribe to DMs and zap receipts addressed to the bot
	// kind:4 = NIP-04 legacy DMs (deprecated but widely used)
	// kind:1059 = NIP-17 gift-wrapped DMs
	// kind:9735 = zap receipts
	filter := nostr.Filter{
		Kinds: []int{nostr.KindEncryptedDirectMessage, nostr.KindGiftWrap, nostr.KindZap},
		Tags:  nostr.TagMap{"p": []string{rm.botPubkeyHex}},
	}

	// Apply since filter if we have a high water mark
	// NIP-01: since is inclusive (>=), so add 1 to exclude already-processed events
	if since > 0 {
		sinceTs := nostr.Timestamp(since + 1)
		filter.Since = &sinceTs
		log.Printf("filtering events after %s", time.Unix(since, 0).Format("2006/01/02 15:04:05"))
	}

	events := rm.pool.SubscribeMany(ctx, rm.relayURLs, filter)

	// Router goroutine: dispatch events by kind to separate channels
	go func() {
		for re := range events {
			switch re.Kind {
			case nostr.KindEncryptedDirectMessage, nostr.KindGiftWrap: // DMs: kind:4 (NIP-04) or kind:1059 (NIP-17 gift-wrapped)
				select {
				case rm.dmEvents <- re.Event:
				default:
					log.Printf("DM event channel full, dropping event %s", re.ID)
				}
			case nostr.KindZap: // Zap receipt
				select {
				case rm.zapEvents <- re.Event:
				default:
					log.Printf("zap event channel full, dropping event %s", re.ID)
				}
			}
		}
		close(rm.dmEvents)
		close(rm.zapEvents)
	}()

	log.Printf("subscribed to %d relays", len(rm.relayURLs))
	return nil
}

// DMEvents returns a channel of gift-wrapped DM events (kind:1059).
func (rm *RelayManager) DMEvents() <-chan *nostr.Event {
	return rm.dmEvents
}

// ZapEvents returns a channel of zap receipt events (kind:9735).
func (rm *RelayManager) ZapEvents() <-chan *nostr.Event {
	return rm.zapEvents
}

// Publish sends an event to all connected relays.
func (rm *RelayManager) Publish(ctx context.Context, event *nostr.Event) error {
	var lastErr error
	var published int

	for result := range rm.pool.PublishMany(ctx, rm.relayURLs, *event) {
		if result.Error != nil {
			lastErr = result.Error
			log.Printf("publish to %s failed: %v", result.RelayURL, result.Error)
			continue
		}
		published++
	}

	if published == 0 {
		return fmt.Errorf("failed to publish to any relay: %w", lastErr)
	}

	log.Printf("published event %s to %d relays", event.ID, published)
	return nil
}

// Close gracefully shuts down all relay connections.
func (rm *RelayManager) Close() {
	if rm.cancel != nil {
		rm.cancel()
	}
	if rm.pool != nil {
		rm.pool.Close("relay manager closed")
	}
	log.Printf("relay manager closed")
}
