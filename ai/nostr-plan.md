# Nostr Package Refactoring Plan

Replace bespoke relay connection/reconnection logic with go-nostr's `SimplePool` abstraction.

## Current State Analysis

### Files in `internal/nostr/`

| File | Purpose | LOC | Status |
|------|---------|-----|--------|
| `relay.go` | RelayManager with manual connection/subscription/reconnection | 240 | Replace |
| `events.go` | EventDeduplicator, EventMultiplexer | 124 | Delete (unused) |

### Current RelayManager Interface (relay.go)

```go
type RelayManager struct { ... }

func NewRelayManager(relayURLs []string, botPubkeyHex string) *RelayManager
func (rm *RelayManager) Connect(ctx context.Context) error
func (rm *RelayManager) DMEvents() <-chan *nostr.Event
func (rm *RelayManager) ZapEvents() <-chan *nostr.Event
func (rm *RelayManager) Publish(ctx context.Context, event *nostr.Event) error
func (rm *RelayManager) Close()
```

### Consumers (in `internal/cli/run.go`)

1. **Line 79**: `relayMgr := nostr.NewRelayManager(cfg.Nostr.Relays, cfg.Nostr.BotPubkeyHex)`
2. **Line 80**: `relayMgr.Connect(ctx)`
3. **Line 83**: `defer relayMgr.Close()`
4. **Line 94**: `<-relayMgr.DMEvents()` - receives kind:1059 events
5. **Line 153**: `<-relayMgr.ZapEvents()` - receives kind:9735 events
6. **Line 196**: `relayMgr.Publish(ctx, wrapped)`
7. **Line 189**: `sendResponse()` takes `*nostr.RelayManager`

### Current Implementation Problems

1. **Reimplements SimplePool functionality**:
   - Per-relay goroutines with reconnection (lines 73-133)
   - Exponential backoff (lines 138-161)
   - Manual relay list management with mutex

2. **Missing SimplePool features**:
   - `WithPenaltyBox()` - smarter connection failure handling
   - Built-in deduplication via `seenAlready` map
   - Auth handling (`WithAuthHandler`)
   - URL normalization

3. **Dead code**: `events.go` defines `EventDeduplicator` and `EventMultiplexer` but neither is imported anywhere.

## go-nostr SimplePool Capabilities

From `ai/context/go-nostr/pool.go`:

| Feature | Method | Notes |
|---------|--------|-------|
| Multi-relay subscription | `SubscribeMany(ctx, urls, filter)` | Built-in reconnect, dedup |
| Multi-relay publish | `PublishMany(ctx, urls, event)` | Returns channel of results |
| Connection management | `EnsureRelay(url)` | Auto-reconnect |
| Penalty box | `WithPenaltyBox()` | Exponential backoff on failures |
| Event middleware | `WithEventMiddleware(fn)` | Process all events |
| Auth handling | `WithAuthHandler(fn)` | NIP-42 auth support |

Key insight: `subMany()` (lines 414-581) contains:
- Automatic reconnection with `goto reconnect` pattern
- Exponential backoff: `interval = min(time.Minute*5, interval*17/10)`
- Built-in deduplication via `seenAlready` xsync map
- EOSE handling
- Since-filter update on reconnect (line 526)

## Refactoring Plan

### Phase 1: Rewrite relay.go

Replace `RelayManager` implementation while preserving the interface.

**New structure**:
```go
type RelayManager struct {
    pool         *nostr.SimplePool
    relayURLs    []string
    botPubkeyHex string
    dmEvents     chan *nostr.Event
    zapEvents    chan *nostr.Event
    cancel       context.CancelFunc
}
```

**Implementation changes**:

| Method | Current | New |
|--------|---------|-----|
| `NewRelayManager()` | Creates struct with relay URLs | Same |
| `Connect()` | Manual per-relay connect + subscribe goroutines | Create `SimplePool` with `WithPenaltyBox()`, call `SubscribeMany()`, start router goroutine |
| `DMEvents()` | Return channel | Same |
| `ZapEvents()` | Return channel | Same |
| `Publish()` | Manual loop over relays | Use `pool.PublishMany()` |
| `Close()` | Cancel context, wait, close relays | Call `pool.Close()`, close channels |

**Router goroutine**:
```go
go func() {
    for re := range pool.SubscribeMany(ctx, urls, filter) {
        switch re.Event.Kind {
        case 1059:
            rm.dmEvents <- re.Event
        case 9735:
            rm.zapEvents <- re.Event
        }
    }
    close(rm.dmEvents)
    close(rm.zapEvents)
}()
```

### Phase 2: Delete events.go

File contains unused code:
- `EventDeduplicator` - not imported
- `EventMultiplexer` - not imported

SimplePool handles deduplication internally.

### Phase 3: Verify

1. Run `go build ./...`
2. Run `go test ./...`
3. Manual test with actual relays (if available)

## Interface Preservation

The external interface remains identical:

```go
// No changes to function signatures
func NewRelayManager(relayURLs []string, botPubkeyHex string) *RelayManager
func (rm *RelayManager) Connect(ctx context.Context) error
func (rm *RelayManager) DMEvents() <-chan *nostr.Event
func (rm *RelayManager) ZapEvents() <-chan *nostr.Event
func (rm *RelayManager) Publish(ctx context.Context, event *nostr.Event) error
func (rm *RelayManager) Close()
```

No changes required to `run.go` or any other consumer.

## Risk Assessment

| Risk | Mitigation |
|------|------------|
| SimplePool reconnection differs from current | SimplePool's reconnection is more robust (tested in production by go-nostr users) |
| Channel semantics change | Preserve exact channel behavior (buffered, close on shutdown) |
| Missing auth support | Current impl doesn't support auth either; SimplePool adds capability for future |

## Lines of Code Impact

| File | Before | After | Delta |
|------|--------|-------|-------|
| relay.go | 240 | ~80 | -160 |
| events.go | 124 | 0 | -124 |
| **Total** | 364 | ~80 | **-284** |

## Execution Checklist

- [ ] Rewrite `relay.go` using SimplePool
- [ ] Delete `events.go`
- [ ] Run `go build ./...`
- [ ] Run `go test ./...`
- [ ] Update `ai/status.md`
