# Status: eggbot

Daily work log. Add entries under date headers (## YYYY-MM-DD) after each unit of work.

See `docs/operations/status-spec.md` for format specification.

## 2025-12-13

### Phase 1: Project Scaffolding
- Created project via `/go-cli-project` with Nostr support
- Customized config schema (Viper + env var binding for EGGBOT_NSEC)
- Created migration `001_initial.sql` with inventory, customers, orders, transactions tables
- Added db package with Open, Migrate, Close methods

### Phase 2: Relay Connection & Subscriptions
- Created `internal/nostr/relay.go` - RelayManager with connect/reconnect/close
- Implemented dual subscription: kind:1059 (DMs) and kind:9735 (zaps)
- Event deduplication by ID
- Graceful shutdown on context cancellation

### Phase 3: DM Decryption & Command Parsing
- Created `internal/dm/unwrap.go` - NIP-17 gift unwrap using go-nostr nip59
- Created `internal/commands/parse.go` - Command parser with customer/admin command types
- Created `internal/commands/permissions.go` - Access control (admin list, customer whitelist)

### Phase 4: Command Execution
- Created `internal/commands/customer_commands.go` - inventory, order, balance, history, help
- Created `internal/commands/admin_commands.go` - add, deliver, payment, adjust, customers, addcustomer, removecustomer
- Created `internal/commands/dispatch.go` - Command dispatcher
- Order pricing: sats_per_half_dozen per 6 eggs (rounded up)
- Constraint: one pending order per customer

### Phase 5: Zap Processing & Response Sending
- Created `internal/zaps/validate.go` - NIP-57 zap receipt validation
- BOLT11 invoice parsing with multiplier support (m/u/n/p)
- Created `internal/zaps/processor.go` - Process zaps, credit balance, auto-mark orders paid
- Created `internal/dm/wrap.go` - NIP-17 response wrapping via nip59.GiftWrap
- Integrated zap handling and DM response sending in run.go
- 44 tests passing with race detector, lint clean

### Phase 6: Systemd Service & Documentation
- Created `eggbot.service` - systemd unit with security hardening
- Created `README.md` - Installation, configuration, commands, troubleshooting
- All 44 tests passing, build successful

## 2025-12-14

### SimplePool Refactor
- Rewrote `internal/nostr/relay.go` to use go-nostr's `SimplePool` abstraction
- Replaced bespoke per-relay goroutines and reconnection logic with `SubscribeMany()`
- Replaced manual publish loop with `PublishMany()`
- Enabled `WithPenaltyBox()` for exponential backoff on connection failures
- Deleted `internal/nostr/events.go` (124 LOC dead code - EventDeduplicator and EventMultiplexer never imported)
- LOC reduction: 364 → 116 (~248 lines removed)
- Interface preserved: no changes required to consumers in run.go
- Build successful, all tests passing

### Protocol-Symmetric DM Responses (2025-12-14)
**End Goal**: Make eggbot respond to incoming DMs using the same protocol (NIP-04 vs NIP-17) as the sender used.

**Implementation Complete**:
- Added `DMProtocol` type and constants (`ProtocolNIP04`, `ProtocolNIP17`) to `internal/dm/wrap.go`
- Implemented `WrapLegacyResponse()` function for NIP-04 encrypted DM responses (kind:4)
  - Uses `nip04.ComputeSharedSecret()` and `nip04.Encrypt()` for encryption
  - Manually builds, signs, and returns the event
  - Mirrors `WrapResponse()` for NIP-17 but with symmetric crypto instead of gift-wrap
- Added comprehensive tests: `TestWrapLegacyResponse()` and `TestWrapLegacyResponse_CanBeDecrypted()`
- Tracked protocol in event loop switch statement:
  - `gonostr.KindEncryptedDirectMessage` → `dm.ProtocolNIP04`
  - `gonostr.KindGiftWrap` → `dm.ProtocolNIP17`
- Modified `sendResponse()` signature to accept `botSecretHex` and `protocol` parameters
- Dispatcher switch in `sendResponse()` routes to appropriate wrapping function
- Updated all three call sites in `internal/cli/run.go` (unknown command, permission denied, command result)
- Replaced hardcoded integer literals with go-nostr constants throughout:
  - `internal/nostr/relay.go` filter definition: 4/1059/9735 → constants
  - `internal/nostr/relay.go` switch statement: 4/1059/9735 → constants
  - `internal/cli/run.go` switch statement: 4/1059 → constants

**Verification**:
- Unit tests: `go test ./internal/dm/...` passes all 5 tests
- Full suite: `go test ./...` passes all tests
- Build: `go build ./...` clean, executable generated successfully
- No code regressions; constants eliminate magic numbers improving maintainability

### Amethyst Markdown Comment Fix
- Amethyst prepends `[//]: # (nip18)` markdown comments to DM content
- Parser now strips lines starting with `[//]:` before extracting command
- Added `stripMarkdownComments()` helper function
- Added 4 test cases covering various markdown comment scenarios
- All tests passing

### High Water Mark for Event Deduplication
- Added migration `003_high_water_mark.sql` with single-row table for tracking last processed event timestamp
- Added `GetHighWaterMark()` and `SetHighWaterMark()` methods to db package
- `SetHighWaterMark()` only updates if new timestamp is greater than current (never goes backward)
- Modified `RelayManager.Connect()` to accept `since` parameter for filtering subscription
- Event loop now updates high water mark after processing each event (DM or zap)
- High water mark updated on ALL outcomes: success, failure, or ignored messages
- On restart, bot only receives events newer than the high water mark
- Added `TestHighWaterMark` covering initial value, forward updates, and no-backward-update behavior
- All tests passing, build successful

### Event ID Deduplication via INSERT OR IGNORE
- Added migration `004_processed_events.sql` with event_id as PRIMARY KEY
- Added `TryProcess(eventID, kind, createdAt)` method to db package
- Uses `INSERT OR IGNORE` for atomic deduplication - returns true (new event) or false (duplicate)
- Integrated dedup check at top of both DM and zap event handlers in run.go
- Same event arriving from multiple relays now processed exactly once
- Complements high water mark (timestamp-based) with event ID-based deduplication
- Added `TestTryProcess` covering first call, duplicate call, and different event scenarios
- All tests passing with race detector, build successful

## 2025-12-15

### Inventory Audit & System Sanity Implementation

Completed 5-phase implementation to fix logical inconsistencies in inventory/order management.

**Phase 1: Cancel Command**
- Added `CmdCancel` constant and routing in parse.go, dispatch.go
- Implemented `CancelOrderCmd()` in customer_commands.go with ownership verification
- Added `CancelOrder()` to operations.go - cancels pending orders, restores reserved inventory
- Added `ErrOrderNotPending` error type
- Added tests: `TestCancelOrder`, `TestCancelOrderCmd`, `TestCancelOrderCmd_OwnershipCheck`

**Phase 2: Delivery Payment Validation**
- Modified `DeliverCmd()` to only fulfill orders with status='paid'
- Added `GetPaidOrdersByCustomer()` to operations.go
- Pending (unpaid) orders are now skipped during delivery
- Updated tests: `TestDeliverCmd`, `TestDeliverCmd_OnlyDeliversPaidOrders`

**Phase 3: Inventory Reservation Model**
- Modified `CreateOrder()` to atomically reserve inventory at order time
- Modified `FulfillOrder()` to NOT deduct inventory (already reserved)
- Modified `CancelOrder()` to restore inventory when cancelling
- Modified `OrderCmd()` message to reflect "eggs reserved" instead of "order created"
- Updated all tests that create orders to add inventory first

**Phase 4: Admin Orders List Command**
- Added `CmdOrders` constant and routing
- Added `OrderWithCustomer` struct and `GetAllOrders()` to operations.go
- Implemented `OrdersCmd()` - lists all orders across all customers with truncated npubs
- Added `TestOrdersCmd` test
- Updated help text with new commands

**Phase 5: Verification**
- All tests passing with race detector (160 test cases)
- Build successful
- Lint warnings in test fixed

**Summary of Bug Fixes**:
| Issue | Status |
|-------|--------|
| Inventory overbooking | FIXED - Reserved at order time |
| Unverified delivery | FIXED - Only paid orders delivered |
| No order cancellation | FIXED - Cancel command added |
| No admin visibility | FIXED - Orders list command added |
| Cancel doesn't restore inventory | FIXED - Atomic restore on cancel |
