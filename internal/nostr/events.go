package nostr

import (
	"sync"
	"time"

	"github.com/nbd-wtf/go-nostr"
)

// EventDeduplicator filters duplicate events that arrive from multiple relays.
// Events are deduplicated by ID and expired after a configurable TTL.
type EventDeduplicator struct {
	seen map[string]time.Time
	ttl  time.Duration
	mu   sync.RWMutex
}

// NewEventDeduplicator creates a deduplicator with the given TTL.
// Events older than TTL are eligible for cleanup.
func NewEventDeduplicator(ttl time.Duration) *EventDeduplicator {
	ed := &EventDeduplicator{
		seen: make(map[string]time.Time),
		ttl:  ttl,
	}
	return ed
}

// IsDuplicate returns true if this event ID has been seen before.
// If not a duplicate, marks the event as seen.
func (ed *EventDeduplicator) IsDuplicate(event *nostr.Event) bool {
	ed.mu.Lock()
	defer ed.mu.Unlock()

	if _, exists := ed.seen[event.ID]; exists {
		return true
	}

	ed.seen[event.ID] = time.Now()
	return false
}

// Cleanup removes entries older than TTL. Call periodically.
func (ed *EventDeduplicator) Cleanup() {
	ed.mu.Lock()
	defer ed.mu.Unlock()

	cutoff := time.Now().Add(-ed.ttl)
	for id, seenAt := range ed.seen {
		if seenAt.Before(cutoff) {
			delete(ed.seen, id)
		}
	}
}

// StartCleanupLoop runs cleanup at regular intervals until context is done.
func (ed *EventDeduplicator) StartCleanupLoop(done <-chan struct{}, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-done:
			return
		case <-ticker.C:
			ed.Cleanup()
		}
	}
}

// EventMultiplexer combines events from multiple channels into one,
// with deduplication.
type EventMultiplexer struct {
	dedup  *EventDeduplicator
	output chan *nostr.Event
	done   chan struct{}
	wg     sync.WaitGroup
}

// NewEventMultiplexer creates a multiplexer with the given buffer size and TTL.
func NewEventMultiplexer(bufferSize int, ttl time.Duration) *EventMultiplexer {
	return &EventMultiplexer{
		dedup:  NewEventDeduplicator(ttl),
		output: make(chan *nostr.Event, bufferSize),
		done:   make(chan struct{}),
	}
}

// AddSource starts forwarding events from the given channel to the output.
func (em *EventMultiplexer) AddSource(source <-chan *nostr.Event) {
	em.wg.Add(1)
	go func() {
		defer em.wg.Done()
		for {
			select {
			case <-em.done:
				return
			case event, ok := <-source:
				if !ok {
					return
				}
				if !em.dedup.IsDuplicate(event) {
					select {
					case em.output <- event:
					case <-em.done:
						return
					}
				}
			}
		}
	}()
}

// Events returns the deduplicated output channel.
func (em *EventMultiplexer) Events() <-chan *nostr.Event {
	return em.output
}

// Close stops all source forwarding and closes the output channel.
func (em *EventMultiplexer) Close() {
	close(em.done)
	em.wg.Wait()
	close(em.output)
}
