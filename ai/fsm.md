# eggbot Finite State Machines

This document describes the three FSMs that govern eggbot's behavior, their integration points, and how to extend them.

## Overview

eggbot uses `github.com/looplab/fsm` to implement explicit state machines that validate state transitions before execution. The database remains the source of truth; FSMs act as validators.

```
internal/fsm/
├── fsm.go           # Shared constants (states, events)
├── order.go         # Order lifecycle FSM
├── inventory.go     # Inventory state FSM
└── processor.go     # Bot event processing FSM
```

## 1. Order State Machine

**Purpose**: Validates order state transitions through the purchase lifecycle.

**Location**: `internal/fsm/order.go`

**Integration**: `internal/db/operations.go` (UpdateOrderStatus, FulfillOrder, CancelOrder)

### State Diagram

```
                    ┌──────────┐
                    │ pending  │
                    └────┬─────┘
                         │
           ┌─────────────┼─────────────┐
           │ [pay]       │             │ [cancel]
           ▼             │             ▼
      ┌─────────┐        │       ┌───────────┐
      │  paid   │        │       │ cancelled │
      └────┬────┘        │       └───────────┘
           │             │
           │ [fulfill]   │
           ▼             │
      ┌───────────┐      │
      │ fulfilled │      │
      └───────────┘      │
```

### States

| State | Description | Next States |
|-------|-------------|-------------|
| `pending` | Order created, awaiting payment | paid, cancelled |
| `paid` | Payment received, ready for delivery | fulfilled |
| `fulfilled` | Order delivered to customer | (terminal) |
| `cancelled` | Order cancelled by customer | (terminal) |

### Events

| Event | From | To | Trigger |
|-------|------|-----|---------|
| `pay` | pending | paid | Zap payment received and applied |
| `fulfill` | paid | fulfilled | Admin marks order delivered |
| `cancel` | pending | cancelled | Customer cancels unpaid order |

### Usage

```go
import "github.com/buildtall-systems/eggbot/internal/fsm"

var orderSM = fsm.NewOrderStateMachine()

// Check if transition is valid
if !orderSM.CanTransition(order.Status, fsm.OrderEventFulfill) {
    return fmt.Errorf("cannot fulfill order in %s state", order.Status)
}

// Execute transition (validates and returns new state)
newState, err := orderSM.Transition(ctx, order.Status, fsm.OrderEventFulfill)
if err != nil {
    return err
}
```

### Adding New Order States

1. Add state constant to `internal/fsm/fsm.go`:
   ```go
   const OrderStateRefunded = "refunded"
   ```

2. Add event constant:
   ```go
   const OrderEventRefund = "refund"
   ```

3. Add transition to `internal/fsm/order.go` in `NewOrderStateMachine()`:
   ```go
   {Name: OrderEventRefund, Src: []string{OrderStatePaid}, Dst: OrderStateRefunded},
   ```

4. Update database operations in `internal/db/operations.go`:
   ```go
   func (db *DB) RefundOrder(ctx context.Context, orderID int64) error {
       order, err := db.GetOrderByID(ctx, orderID)
       if err != nil { return err }

       if !orderSM.CanTransition(order.Status, fsm.OrderEventRefund) {
           return fmt.Errorf("%w: cannot refund order in %s state",
               ErrInvalidStateTransition, order.Status)
       }

       // Execute with atomic WHERE
       result, err := db.ExecContext(ctx, `
           UPDATE orders SET status = 'refunded', updated_at = CURRENT_TIMESTAMP
           WHERE id = ? AND status = 'paid'
       `, orderID)
       // ... check rows affected
   }
   ```

5. Add tests in `internal/fsm/order_test.go`:
   ```go
   func TestOrderStateMachine_Refund(t *testing.T) {
       osm := NewOrderStateMachine()
       ctx := context.Background()

       // Valid: paid → refunded
       newState, err := osm.Transition(ctx, OrderStatePaid, OrderEventRefund)
       if err != nil { t.Fatal(err) }
       if newState != OrderStateRefunded { t.Errorf("got %s", newState) }

       // Invalid: pending → refunded
       _, err = osm.Transition(ctx, OrderStatePending, OrderEventRefund)
       if err == nil { t.Error("expected error") }
   }
   ```

6. Update `inferOrderEvent()` in operations.go if needed.

## 2. Inventory State Machine

**Purpose**: Validates inventory state transitions during order operations.

**Location**: `internal/fsm/inventory.go`

**Integration**: Conceptual model for inventory flow; actual counts managed in database.

### State Diagram

```
      ┌───────────┐
      │ available │◄────────────┐
      └─────┬─────┘             │
            │                   │
            │ [reserve]         │ [restore]
            ▼                   │
      ┌───────────┐             │
      │ reserved  │─────────────┘
      └─────┬─────┘
            │
            │ [consume]
            ▼
      ┌───────────┐
      │ consumed  │
      └───────────┘
```

### States

| State | Description | Next States |
|-------|-------------|-------------|
| `available` | Eggs in stock | reserved |
| `reserved` | Eggs reserved for pending order | available (restore), consumed |
| `consumed` | Eggs delivered to customer | (terminal) |

### Events

| Event | From | To | Trigger |
|-------|------|-----|---------|
| `reserve` | available | reserved | Order created |
| `restore` | reserved | available | Order cancelled |
| `consume` | reserved | consumed | Order fulfilled |

### Usage

```go
import "github.com/buildtall-systems/eggbot/internal/fsm"

var inventorySM = fsm.NewInventoryStateMachine()

// Check reserve operation
if !inventorySM.CanReserve() {
    return fmt.Errorf("cannot reserve from current state")
}

// Check restore operation (for cancellation)
if !inventorySM.CanRestore() {
    return fmt.Errorf("cannot restore inventory")
}
```

### Inventory Flow in Database Operations

```
CreateOrder:   inventory -= quantity (reserve)
CancelOrder:   inventory += quantity (restore)
FulfillOrder:  (no change - already reserved)
```

### Adding New Inventory States

1. Add state/event constants to `internal/fsm/fsm.go`:
   ```go
   const InventoryStateExpired = "expired"
   const InventoryEventExpire = "expire"
   ```

2. Add transition to `internal/fsm/inventory.go`:
   ```go
   {Name: InventoryEventExpire, Src: []string{InventoryStateReserved}, Dst: InventoryStateExpired},
   ```

3. Add helper method:
   ```go
   func (is *InventoryStateMachine) CanExpire() bool {
       is.mu.Lock()
       defer is.mu.Unlock()
       is.fsm.SetState(InventoryStateReserved)
       return is.fsm.Can(InventoryEventExpire)
   }
   ```

4. Add tests in `internal/fsm/inventory_test.go`.

## 3. Event Processor FSM

**Purpose**: Tracks bot's event processing state through the DM/zap handling pipeline.

**Location**: `internal/fsm/processor.go`

**Integration**: `internal/cli/run.go` (main event loop)

### State Diagram

```
                              ┌─────────────────────────────────┐
                              │           [error]               │
                              ▼                                 │
┌──────┐  [dm_received]  ┌──────────────┐  [command_processed]  │
│ idle │────────────────►│processing_dm │──────────────────────►│
└──┬───┘                 └──────────────┘                       │
   │                                                            │
   │                                                            │
   │  [zap_received]     ┌───────────────┐                      │
   └────────────────────►│processing_zap │──────────────────────┤
                         └───────────────┘                      │
                                                                │
                              ┌──────────────────┐              │
                              │ sending_response │◄─────────────┘
                              └────────┬─────────┘
                                       │
                                       │ [response_sent]
                                       ▼
                                   ┌──────┐
                                   │ idle │
                                   └──────┘
```

### States

| State | Description | Next States |
|-------|-------------|-------------|
| `idle` | Waiting for events | processing_dm, processing_zap |
| `processing_dm` | Processing DM command | sending_response, idle (error) |
| `processing_zap` | Processing zap payment | sending_response, idle (error) |
| `sending_response` | Sending DM response | idle |

### Events

| Event | From | To | Trigger |
|-------|------|-----|---------|
| `dm_received` | idle | processing_dm | DM event received from relay |
| `zap_received` | idle | processing_zap | Zap event received from relay |
| `command_processed` | processing_dm | sending_response | Command executed |
| `response_sent` | sending_response, processing_zap | idle | Response sent to user |
| `error` | processing_dm, processing_zap, sending_response | idle | Error occurred |

### Usage in Event Loop

```go
// Initialize at startup
processorFSM := fsm.NewEventProcessorFSM()

// DM received
if err := processorFSM.Event(ctx, fsm.ProcessorEventDMReceived); err != nil {
    log.Printf("FSM error: %v", err)
    processorFSM.Reset()
    continue
}

// ... process command ...

// Command processed
if err := processorFSM.Event(ctx, fsm.ProcessorEventCommandProcessed); err != nil {
    processorFSM.Reset()
    continue
}

// ... send response ...

// Response sent
if err := processorFSM.Event(ctx, fsm.ProcessorEventResponseSent); err != nil {
    processorFSM.Reset()
    continue
}

// Reset for next event
processorFSM.Reset()
```

### Error Handling Pattern

```go
if result.Error != nil {
    // Transition to error state
    _ = processorFSM.Event(ctx, fsm.ProcessorEventError)

    // Send error response
    sendResponse(ctx, ..., fmt.Sprintf("Error: %v", result.Error), ...)

    // Reset to idle
    processorFSM.Reset()
    continue
}
```

### Adding New Processor States

Example: Adding a "queued" state for holding events during rate limiting.

1. Add constants to `internal/fsm/fsm.go`:
   ```go
   const ProcessorStateQueued = "queued"
   const ProcessorEventQueue = "queue"
   const ProcessorEventDequeue = "dequeue"
   ```

2. Add transitions to `internal/fsm/processor.go`:
   ```go
   {Name: ProcessorEventQueue, Src: []string{ProcessorStateIdle}, Dst: ProcessorStateQueued},
   {Name: ProcessorEventDequeue, Src: []string{ProcessorStateQueued}, Dst: ProcessorStateIdle},
   ```

3. Update event loop in `internal/cli/run.go`:
   ```go
   // Check if rate limited
   if rateLimited {
       if err := processorFSM.Event(ctx, fsm.ProcessorEventQueue); err != nil {
           log.Printf("FSM queue error: %v", err)
       }
       eventQueue.Add(event)
       continue
   }
   ```

4. Add tests in `internal/fsm/processor_test.go`.

### Registering Callbacks

```go
processorFSM := fsm.NewEventProcessorFSM()

// Log state entries
processorFSM.OnEnter(fsm.ProcessorStateProcessingDM, func() {
    log.Printf("Started processing DM")
    metrics.DMProcessingStarted.Inc()
})

// Log state exits
processorFSM.OnLeave(fsm.ProcessorStateIdle, func() {
    log.Printf("Bot no longer idle")
})
```

## Design Patterns

### FSM as Validator

The FSM validates transitions but doesn't own state:

```go
// FSM validates the transition is legal
if !orderSM.CanTransition(order.Status, fsm.OrderEventFulfill) {
    return ErrInvalidStateTransition
}

// Database owns the actual state
result, err := db.ExecContext(ctx, `
    UPDATE orders SET status = 'fulfilled'
    WHERE id = ? AND status = 'paid'  -- Atomic precondition
`, orderID)

// Check if precondition still held
if result.RowsAffected() == 0 {
    return fmt.Errorf("order state changed concurrently")
}
```

### Thread Safety

All FSMs use mutex-protected singletons:

```go
type OrderStateMachine struct {
    fsm *fsm.FSM
    mu  sync.Mutex
}

func (os *OrderStateMachine) CanTransition(src, event string) bool {
    os.mu.Lock()
    defer os.mu.Unlock()
    os.fsm.SetState(src)
    return os.fsm.Can(event)
}
```

### Atomic WHERE Clauses

Prevent race conditions by checking preconditions atomically:

```go
// BAD: Check-then-act race condition
if order.Status == "paid" {
    db.Exec("UPDATE orders SET status = 'fulfilled' WHERE id = ?", id)
}

// GOOD: Atomic precondition check
result := db.Exec(`
    UPDATE orders SET status = 'fulfilled'
    WHERE id = ? AND status = 'paid'
`, id)
if result.RowsAffected() == 0 {
    return errors.New("order not in paid state")
}
```

## Testing FSMs

### Test Valid Transitions

```go
func TestOrderStateMachine_ValidTransitions(t *testing.T) {
    osm := NewOrderStateMachine()
    ctx := context.Background()

    tests := []struct {
        from, event, to string
    }{
        {OrderStatePending, OrderEventPay, OrderStatePaid},
        {OrderStatePaid, OrderEventFulfill, OrderStateFulfilled},
        {OrderStatePending, OrderEventCancel, OrderStateCancelled},
    }

    for _, tt := range tests {
        newState, err := osm.Transition(ctx, tt.from, tt.event)
        if err != nil {
            t.Errorf("%s + %s: %v", tt.from, tt.event, err)
        }
        if newState != tt.to {
            t.Errorf("got %s, want %s", newState, tt.to)
        }
    }
}
```

### Test Invalid Transitions

```go
func TestOrderStateMachine_InvalidTransitions(t *testing.T) {
    osm := NewOrderStateMachine()
    ctx := context.Background()

    tests := []struct {
        from, event string
    }{
        {OrderStatePending, OrderEventFulfill},  // Can't fulfill unpaid
        {OrderStatePaid, OrderEventCancel},       // Can't cancel paid
        {OrderStateFulfilled, OrderEventPay},     // Terminal state
    }

    for _, tt := range tests {
        _, err := osm.Transition(ctx, tt.from, tt.event)
        if err == nil {
            t.Errorf("%s + %s should fail", tt.from, tt.event)
        }
    }
}
```

### Test Concurrent Access

```go
func TestOrderStateMachine_ConcurrentAccess(t *testing.T) {
    osm := NewOrderStateMachine()
    ctx := context.Background()
    var wg sync.WaitGroup

    for i := 0; i < 100; i++ {
        wg.Add(1)
        go func() {
            defer wg.Done()
            osm.CanTransition(OrderStatePending, OrderEventPay)
            _, _ = osm.Transition(ctx, OrderStatePending, OrderEventPay)
        }()
    }

    wg.Wait()
}
```

## Constants Reference

### Order FSM

```go
// States
fsm.OrderStatePending    // "pending"
fsm.OrderStatePaid       // "paid"
fsm.OrderStateFulfilled  // "fulfilled"
fsm.OrderStateCancelled  // "cancelled"

// Events
fsm.OrderEventPay        // "pay"
fsm.OrderEventFulfill    // "fulfill"
fsm.OrderEventCancel     // "cancel"
```

### Inventory FSM

```go
// States
fsm.InventoryStateAvailable  // "available"
fsm.InventoryStateReserved   // "reserved"
fsm.InventoryStateConsumed   // "consumed"

// Events
fsm.InventoryEventReserve    // "reserve"
fsm.InventoryEventConsume    // "consume"
fsm.InventoryEventRestore    // "restore"
```

### Processor FSM

```go
// States
fsm.ProcessorStateIdle            // "idle"
fsm.ProcessorStateProcessingDM    // "processing_dm"
fsm.ProcessorStateProcessingZap   // "processing_zap"
fsm.ProcessorStateSendingResponse // "sending_response"

// Events
fsm.ProcessorEventDMReceived       // "dm_received"
fsm.ProcessorEventZapReceived      // "zap_received"
fsm.ProcessorEventCommandProcessed // "command_processed"
fsm.ProcessorEventResponseSent     // "response_sent"
fsm.ProcessorEventError            // "error"
```

## Files

| File | Purpose |
|------|---------|
| `internal/fsm/fsm.go` | Shared constants |
| `internal/fsm/order.go` | OrderStateMachine |
| `internal/fsm/order_test.go` | Order FSM tests |
| `internal/fsm/inventory.go` | InventoryStateMachine |
| `internal/fsm/inventory_test.go` | Inventory FSM tests |
| `internal/fsm/processor.go` | EventProcessorFSM |
| `internal/fsm/processor_test.go` | Processor FSM tests |
| `internal/db/operations.go` | Order FSM integration |
| `internal/cli/run.go` | Processor FSM integration |
| `docs/FSM_MIGRATION.md` | Comprehensive migration guide |
| `docs/FSM_QUICK_REFERENCE.md` | Quick reference |
