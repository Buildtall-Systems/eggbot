package nostr

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/nbd-wtf/go-nostr"
)

// RelayManager handles connections to multiple Nostr relays and manages subscriptions.
type RelayManager struct {
	relayURLs    []string
	botPubkeyHex string
	relays       []*nostr.Relay
	mu           sync.RWMutex

	// Event channels for consumers
	dmEvents  chan *nostr.Event // kind:1059 gift-wrapped DMs
	zapEvents chan *nostr.Event // kind:9735 zap receipts

	// Internal state
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
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
func (rm *RelayManager) Connect(ctx context.Context) error {
	rm.ctx, rm.cancel = context.WithCancel(ctx)

	var connected int
	for _, url := range rm.relayURLs {
		relay, err := nostr.RelayConnect(rm.ctx, url)
		if err != nil {
			log.Printf("failed to connect to %s: %v", url, err)
			continue
		}

		rm.mu.Lock()
		rm.relays = append(rm.relays, relay)
		rm.mu.Unlock()

		connected++
		log.Printf("connected to %s", url)

		// Start subscription goroutine for this relay
		rm.wg.Add(1)
		go rm.subscribeRelay(relay)
	}

	if connected == 0 {
		return fmt.Errorf("failed to connect to any relays")
	}

	log.Printf("connected to %d/%d relays", connected, len(rm.relayURLs))
	return nil
}

// subscribeRelay manages subscriptions for a single relay with reconnection logic.
func (rm *RelayManager) subscribeRelay(relay *nostr.Relay) {
	defer rm.wg.Done()

	backoff := time.Second
	maxBackoff := 30 * time.Second

	for {
		select {
		case <-rm.ctx.Done():
			return
		default:
		}

		// Create subscription filters
		// kind:1059 = gift-wrapped DMs addressed to the bot
		// kind:9735 = zap receipts (we filter by p-tag for bot's pubkey)
		filters := []nostr.Filter{
			{
				Kinds: []int{1059}, // Gift-wrapped DMs
				Tags:  nostr.TagMap{"p": []string{rm.botPubkeyHex}},
			},
			{
				Kinds: []int{9735}, // Zap receipts
				Tags:  nostr.TagMap{"p": []string{rm.botPubkeyHex}},
			},
		}

		sub, err := relay.Subscribe(rm.ctx, filters)
		if err != nil {
			log.Printf("subscription failed on %s: %v", relay.URL, err)
			if rm.reconnect(relay, &backoff, maxBackoff) {
				continue
			}
			return
		}

		// Reset backoff on successful subscription
		backoff = time.Second
		log.Printf("subscribed to events on %s", relay.URL)

		// Process events from subscription
		for {
			select {
			case <-rm.ctx.Done():
				sub.Unsub()
				return

			case event, ok := <-sub.Events:
				if !ok {
					// Channel closed, relay disconnected
					log.Printf("subscription closed on %s, reconnecting...", relay.URL)
					if rm.reconnect(relay, &backoff, maxBackoff) {
						break // Break inner loop to resubscribe
					}
					return
				}

				rm.routeEvent(event)
			}
		}
	}
}

// reconnect attempts to reconnect to a relay with exponential backoff.
// Returns true if reconnection should be attempted, false if context is done.
func (rm *RelayManager) reconnect(relay *nostr.Relay, backoff *time.Duration, maxBackoff time.Duration) bool {
	select {
	case <-rm.ctx.Done():
		return false
	case <-time.After(*backoff):
	}

	// Attempt reconnection
	err := relay.Connect(rm.ctx)
	if err != nil {
		log.Printf("reconnect to %s failed: %v", relay.URL, err)

		// Increase backoff
		*backoff *= 2
		if *backoff > maxBackoff {
			*backoff = maxBackoff
		}
		return true
	}

	log.Printf("reconnected to %s", relay.URL)
	*backoff = time.Second
	return true
}

// routeEvent sends events to appropriate channels based on kind.
func (rm *RelayManager) routeEvent(event *nostr.Event) {
	switch event.Kind {
	case 1059: // Gift-wrapped DM
		select {
		case rm.dmEvents <- event:
		default:
			log.Printf("DM event channel full, dropping event %s", event.ID)
		}

	case 9735: // Zap receipt
		select {
		case rm.zapEvents <- event:
		default:
			log.Printf("zap event channel full, dropping event %s", event.ID)
		}
	}
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
	rm.mu.RLock()
	relays := make([]*nostr.Relay, len(rm.relays))
	copy(relays, rm.relays)
	rm.mu.RUnlock()

	var lastErr error
	var published int

	for _, relay := range relays {
		err := relay.Publish(ctx, *event)
		if err != nil {
			lastErr = err
			log.Printf("publish to %s failed: %v", relay.URL, err)
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

	rm.wg.Wait()

	rm.mu.Lock()
	for _, relay := range rm.relays {
		_ = relay.Close()
	}
	rm.relays = nil
	rm.mu.Unlock()

	close(rm.dmEvents)
	close(rm.zapEvents)

	log.Printf("relay manager closed")
}
