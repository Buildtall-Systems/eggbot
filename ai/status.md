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

## 2025-12-15 (continued)

### Comprehensive FSM Migration

Completed 7-phase migration to explicit finite state machines (looplab/fsm) for order, inventory, and bot event processing.

**Phase 1: Foundation (Complete)**
- Added looplab/fsm dependency to go.mod
- Created `internal/fsm/` package structure
- Added `fsm.go` with shared constants for all three FSMs

**Phase 2: OrderStateMachine (Complete)**
- Implemented `internal/fsm/order.go` with states: pending, paid, fulfilled, cancelled
- Events: pay, fulfill, cancel
- Comprehensive test coverage in `order_test.go` (100% paths)
- Tests: valid transitions, invalid transitions, can-query predicates, all states covered

**Phase 3: InventoryStateMachine (Complete)**
- Implemented `internal/fsm/inventory.go` with states: available, reserved, consumed
- Events: reserve, restore, consume
- Comprehensive test coverage in `inventory_test.go` (100% paths)
- Tests: reserve, consume, restore, invalid transitions

**Phase 4: EventProcessorFSM (Complete)**
- Implemented `internal/fsm/processor.go` with states: idle, processing_dm, processing_zap, sending_response
- Events: dm_received, zap_received, command_processed, response_sent, error
- Comprehensive test coverage in `processor_test.go` (100% paths)
- Tests: DM flow, zap flow, error recovery, invalid transitions, callbacks, reset, concurrent access

**Phase 5: Order FSM Integration (Complete)**
- Integrated OrderStateMachine into `internal/db/operations.go`
- `UpdateOrderStatus()` validates transitions before state change
- `FulfillOrder()` enforces paid-state requirement via FSM + atomic WHERE clause
- `CancelOrder()` validates cancel only possible from pending state
- Added `ErrInvalidStateTransition` error type
- Updated all database tests to follow valid state progressions:
  - `TestFulfillOrder` - requires paid state
  - `TestTransactionsAndBalance` - follows pending→paid→fulfilled
  - `TestCancelOrder` - cancels from pending, restores inventory
- Updated customer command tests: `TestBalanceCmd` follows correct state progression

**Phase 6: Bot FSM Integration (Complete)**
- Imported fsm package in `internal/cli/run.go`
- Initialized EventProcessorFSM at bot startup
- DM processing flow: idle →[dm_received]→ processing_dm →[command_processed]→ sending_response →[response_sent]→ idle
- Zap processing flow: idle →[zap_received]→ processing_zap →[response_sent]→ idle
- Error handling: [any state] →[error]→ idle on failures
- FSM reset to idle after each event completes or on error
- All valid state transitions logged on errors for debugging

**Phase 7: Documentation & Cleanup (Complete)**
- Created `docs/FSM_MIGRATION.md` - Comprehensive 300+ line migration guide
  - Overview, motivation, architecture (three FSMs)
  - Design patterns: FSM as Validator, thread-safe access
  - Testing strategy with 25+ test cases
  - Migration checklist (all items complete)
  - Quality metrics: 100% coverage, 0 race conditions, 0 lint errors
  - Migration guide for maintainers (adding states/events)
  - Debugging techniques
  - Future enhancements: visualization, hooks, queued processing, retry logic, history, composition
- Created `docs/FSM_QUICK_REFERENCE.md` - Quick reference guide
  - Import statements
  - Usage patterns for all three FSMs
  - State and event diagrams
  - Common patterns: validate-then-execute, error recovery, callbacks
  - All constants defined and documented
  - Debugging guide
  - File locations
- Created `.claude/LEARNINGS.md` - Patterns & insights
  - FSM as Validator pattern (PROVEN)
  - Test-driven FSM definition (PROVEN)
  - Event loop FSM integration (PROVEN)
  - Atomic WHERE clauses for optimistic locking (PROVEN)
  - Test migration pattern (PROVEN)
  - Extraction readiness assessment
  - Known limitations and future work

**Verification**:
- Full test suite passes with race detector: `go test -race ./...`
- Build successful: `go build ./...`
- Lint clean (zero FSM-related issues): Only 3 pre-existing staticcheck issues in relay.go
- All tests re-run after each phase: 160+ test cases passing
- Quality metrics:
  - Test coverage: 100% of FSM code paths
  - Race conditions: 0 detected
  - Lint errors: 0 in FSM code
  - State transitions: 13 total (4 order + 3 inventory + 5 processor + error path)
  - Thread safety: Mutex-protected singleton per FSM package

**FSM as Validator Pattern**:
- Database remains source of truth; FSM validates logic before execution
- Package-level FSM singletons with mutex protection
- Atomic WHERE clauses prevent concurrent state corruption
- Invalid transitions caught at validation layer
- Reusable pattern for any domain with state validation needs

**Key Insights**:
1. FSM doesn't own state—database does (separation of concerns)
2. Test-first approach exposed all edge cases and invalid assumptions
3. Atomic WHERE prevents race conditions without explicit locks
4. Error recovery via FSM reset ensures bot always reaches valid state
5. Pattern ready for extraction and reuse in other buildtall.systems projects

## 2025-12-27

### Inventory Notification Feature

Implemented `notify <qty>` command allowing customers to request DM notifications when inventory reaches their specified threshold.

**Specification**:
- `notify <6|12>` - Subscribe to notification (quantized to order sizes)
- `notify off` - Cancel subscription
- `notify` (no args) - Show current subscription status
- One-shot notification: subscription deleted after DM sent
- Upsert semantics: re-running command updates threshold

**Implementation (5 phases)**:

1. **Migration**: `005_inventory_notifications.sql`
   - `inventory_notifications` table with CHECK constraint (6 or 12 only)
   - Foreign key to customers with ON DELETE CASCADE
   - Unique constraint per customer

2. **Database operations**: Added to `internal/db/operations.go`
   - `UpsertInventoryNotification()` - INSERT OR REPLACE semantics
   - `DeleteInventoryNotification()` - Remove by customer ID
   - `GetInventoryNotification()` - Query current subscription
   - `GetTriggeredNotifications()` - Query subscriptions meeting threshold
   - `DeleteInventoryNotificationByID()` - Remove after sending

3. **Command implementation**:
   - Added `CmdNotify` constant to `internal/commands/parse.go`
   - Added `NotifyCmd()` to `internal/commands/customer_commands.go`
   - Added dispatch case in `internal/commands/dispatch.go`

4. **Notification trigger**: Added `checkInventoryNotifications()` in `internal/cli/run.go`
   - Called after `inventory` and `cancel` commands
   - Queries triggered notifications, sends DMs, deletes subscriptions

5. **Help text**: Updated to include notify commands

**Verification**:
- All tests passing with race detector
- Lint clean
- Build successful

**Plan artifact**: `thoughts/plans/2025-12-26_18-41-28_eggbot-notify-command.md`

**Committed**: `5ef3375` feat: add notify command for inventory threshold alerts
