# FSM Quick Reference

## Import FSM Package

```go
import "github.com/buildtall-systems/eggbot/internal/fsm"
```

## Order State Machine

**Use**: Validate order transitions in database operations

```go
var orderSM = fsm.NewOrderStateMachine()

// Check if transition is possible (before database operation)
if !orderSM.CanTransition(order.Status, fsm.OrderEventFulfill) {
    return fmt.Errorf("cannot fulfill order in %s state", order.Status)
}

// Execute transition (validates precondition is still met)
if _, err := orderSM.Transition(ctx, order.Status, fsm.OrderEventFulfill); err != nil {
    return err
}
```

**State Diagram**:
```
pending --[pay]--> paid --[fulfill]--> fulfilled
   ^                 |
   |                 +----[cancel to pending]--> (not allowed)
   +--[cancel]---------> cancelled
```

**Infer Event from Status**:
```go
event := inferOrderEvent("pending", "paid")  // Returns: OrderEventPay
event := inferOrderEvent("paid", "fulfilled")  // Returns: OrderEventFulfill
```

## Inventory State Machine

**Use**: Validate inventory state transitions during order operations

```go
var inventorySM = fsm.NewInventoryStateMachine()

// Reserve inventory when creating order
if !inventorySM.CanReserve() {
    return fmt.Errorf("cannot reserve inventory")
}

// Restore on cancellation
if !inventorySM.CanRestore() {
    return fmt.Errorf("cannot restore inventory")
}

// Consume on fulfillment
if !inventorySM.CanConsume() {
    return fmt.Errorf("cannot consume inventory")
}
```

**State Diagram**:
```
available --[reserve]--> reserved --[consume]--> consumed
              ^             |
              |             +--[restore]--> available
              +-----[cancel]--^
```

## Event Processor FSM

**Use**: Track bot event processing state in main event loop

```go
processorFSM := fsm.NewEventProcessorFSM()

// DM Processing
if err := processorFSM.Event(ctx, fsm.ProcessorEventDMReceived); err != nil {
    log.Printf("FSM error: %v", err)
    processorFSM.Reset()
    continue
}

if err := processorFSM.Event(ctx, fsm.ProcessorEventCommandProcessed); err != nil {
    log.Printf("FSM error: %v", err)
    processorFSM.Reset()
    continue
}

if err := processorFSM.Event(ctx, fsm.ProcessorEventResponseSent); err != nil {
    log.Printf("FSM error: %v", err)
    processorFSM.Reset()
    continue
}

// Reset to idle when done
processorFSM.Reset()
```

**State Diagram**:
```
                    +--[error]--+
                    |           |
idle --[dm_received]--> processing_dm --[command_processed]--> sending_response --[response_sent]--> idle
 ^                                                                    ^
 |                                                                    |
 +--[zap_received]--> processing_zap --[response_sent]-------+
                                 ^
                                 |
                                 +--[error]--+
```

## Common Patterns

### Validate then Execute Pattern

```go
// Check if transition is possible
if !osm.CanTransition(order.Status, fsm.OrderEventFulfill) {
    return fmt.Errorf("invalid transition")
}

// Execute transition (validates precondition)
if _, err := osm.Transition(ctx, order.Status, fsm.OrderEventFulfill); err != nil {
    return fmt.Errorf("transition failed: %w", err)
}

// Execute database operation with atomic WHERE
result, err := db.ExecContext(ctx, `
    UPDATE orders SET status = 'fulfilled'
    WHERE id = ? AND status = 'paid'
`, orderID)

if result.RowsAffected() == 0 {
    return fmt.Errorf("order no longer in expected state")
}
```

### Error Recovery Pattern

```go
if result.Error != nil {
    // Transition to error state
    if err := processorFSM.Event(ctx, fsm.ProcessorEventError); err != nil {
        log.Printf("FSM error state failed: %v", err)
    }

    // Reset to idle to allow next event
    processorFSM.Reset()

    // Send error response
    sendErrorResponse(err)
    continue
}
```

### Register Callbacks

```go
processorFSM.OnEnter(fsm.ProcessorStateProcessingDM, func() {
    log.Printf("Entered processing_dm state")
})

processorFSM.OnLeave(fsm.ProcessorStateIdle, func() {
    log.Printf("Left idle state")
})
```

## Constants

### Order States
- `OrderStatePending` = "pending"
- `OrderStatePaid` = "paid"
- `OrderStateFulfilled` = "fulfilled"
- `OrderStateCancelled` = "cancelled"

### Order Events
- `OrderEventPay` = "pay"
- `OrderEventFulfill` = "fulfill"
- `OrderEventCancel` = "cancel"

### Inventory States
- `InventoryStateAvailable` = "available"
- `InventoryStateReserved` = "reserved"
- `InventoryStateConsumed` = "consumed"

### Inventory Events
- `InventoryEventReserve` = "reserve"
- `InventoryEventConsume` = "consume"
- `InventoryEventRestore` = "restore"

### Processor States
- `ProcessorStateIdle` = "idle"
- `ProcessorStateProcessingDM` = "processing_dm"
- `ProcessorStateProcessingZap` = "processing_zap"
- `ProcessorStateSendingResponse` = "sending_response"

### Processor Events
- `ProcessorEventDMReceived` = "dm_received"
- `ProcessorEventZapReceived` = "zap_received"
- `ProcessorEventCommandProcessed` = "command_processed"
- `ProcessorEventResponseSent` = "response_sent"
- `ProcessorEventError` = "error"

## Debugging

### Check Current State
```go
currentState := processorFSM.Current()  // Returns string like "idle"
```

### Check If Transition Is Possible
```go
if processorFSM.Can(fsm.ProcessorEventCommandProcessed) {
    log.Printf("Can execute command")
} else {
    log.Printf("Cannot execute command from current state")
}
```

### Manually Set State (Testing Only)
```go
processorFSM.Reset()  // Reset to initial idle state
```

### Log State Transitions
```go
if err := processorFSM.Event(ctx, fsm.ProcessorEventDMReceived); err != nil {
    log.Printf("FSM transition failed: from %s with event %s: %v",
        processorFSM.Current(), fsm.ProcessorEventDMReceived, err)
    return
}
```

## Testing

### Test FSM Directly
```go
func TestMyFSM(t *testing.T) {
    fsm := fsm.NewEventProcessorFSM()
    ctx := context.Background()

    // Test valid transition
    if err := fsm.Event(ctx, fsm.ProcessorEventDMReceived); err != nil {
        t.Fatalf("expected valid transition, got: %v", err)
    }

    // Verify state changed
    if fsm.Current() != fsm.ProcessorStateProcessingDM {
        t.Errorf("expected processing_dm, got %s", fsm.Current())
    }
}
```

## Files

- **Definitions**: `internal/fsm/fsm.go` (state and event constants)
- **Order FSM**: `internal/fsm/order.go`
- **Inventory FSM**: `internal/fsm/inventory.go`
- **Processor FSM**: `internal/fsm/processor.go`
- **Database Integration**: `internal/db/operations.go` (uses OrderStateMachine)
- **Bot Integration**: `internal/cli/run.go` (uses EventProcessorFSM)
- **Tests**: All `*_test.go` files in `internal/fsm/` and `internal/db/`
- **Documentation**: `docs/FSM_MIGRATION.md` (comprehensive guide)

## Links

- [FSM Migration Guide](./FSM_MIGRATION.md)
- [looplab/fsm Library](https://github.com/looplab/fsm)
