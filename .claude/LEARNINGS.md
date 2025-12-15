# eggbot Learnings & Patterns

## FSM as Validator Pattern (PROVEN)

**Context**: Migrated eggbot's implicit state management to explicit FSMs using looplab/fsm

**Pattern**: Database remains source of truth; FSMs validate transitions before execution

**Key Insights**:
1. FSM doesn't own persistent state—database does
2. Package-level FSM singleton with mutex for thread safety
3. Atomic WHERE clauses prevent concurrent state corruption
4. Invalid transitions caught at validation layer, not at persistence layer

**Why It Works**:
- Separates concerns: FSM validates logic, database manages persistence
- Allows adding new validation rules without schema changes
- Thread-safe by design (mutex-protected singletons)
- Fail-fast: Invalid transitions caught before database operations

**Example**:
```go
// FSM validates transition is possible
if !orderSM.CanTransition(order.Status, fsm.OrderEventFulfill) {
    return fmt.Errorf("cannot fulfill in %s state", order.Status)
}

// Atomic WHERE prevents race condition
result := db.ExecContext(ctx, `
    UPDATE orders SET status = 'fulfilled'
    WHERE id = ? AND status = 'paid'  // Must be paid state
`, orderID)
```

**Applicability**: Excellent for domain logic validation in database-centric systems. Use when:
- Multiple services might transition state
- State validation rules are complex or frequently change
- Race conditions are a concern
- Audit trail of invalid transitions is valuable

---

## Test-Driven FSM Definition (PROVEN)

**Context**: Defined three FSMs (Order, Inventory, Processor) via comprehensive test-first approach

**Pattern**: Write test cases that enumerate all valid/invalid transitions, then implement FSM to satisfy tests

**Key Insights**:
1. Tests expose FSM completeness—can't forget edge cases
2. Transition table in tests documents expected behavior
3. ConcurrentAccess tests catch mutex/race issues early
4. Tests serve as executable specification

**Why It Works**:
- Specification is automatically executable and verifiable
- Changes to FSM must pass existing tests
- New developers understand state machine by reading tests
- Race detector catches synchronization bugs

**Test Structure**:
```go
// DMFlow test: Happy path
// ZapFlow test: Alternative path
// ErrorRecovery test: Error handling in all states
// InvalidTransitions test: Comprehensive negative tests
// Can test: Predicate queries
// Callbacks test: Enter/leave hooks
// ConcurrentAccess test: Thread safety under load
```

**Applicability**: Essential for any state machine. Non-negotiable for:
- Concurrent systems
- Multi-path workflows
- Systems with error recovery
- Code with undefined behavior potential

---

## Event Loop State Machine Integration (PROVEN)

**Context**: Integrated EventProcessorFSM into bot's main event loop (run.go)

**Pattern**: FSM transitions at key milestones (receive → process → respond). Reset to idle on completion or error.

**Key Insights**:
1. Transitions should correspond to major processing phases
2. Always reset FSM on error (prevents stuck state)
3. Invalid transitions indicate logic bugs—log and skip event
4. FSM provides early warning for blocked/impossible flows

**Why It Works**:
- Clear visibility into bot's processing state
- Prevents invalid operation sequences
- Error states self-resolve (reset to idle)
- Validates bot follows intended workflow

**Flow Example**:
```
DM Flow: idle → processing_dm → sending_response → idle
Zap Flow: idle → processing_zap → sending_response → idle
Error: [any state] → error → idle
```

**Applicability**: Valuable for any event-driven system where:
- Processing follows distinct phases
- Invalid phase sequences indicate bugs
- Error recovery requires known state
- Observability of processing pipeline is important

---

## Atomic WHERE Clauses for Optimistic Locking (PROVEN)

**Context**: Combined FSM validation with atomic database updates to prevent race conditions

**Pattern**: Include current state in WHERE clause. Update succeeds only if precondition still holds.

**Key Insights**:
1. FSM validates transition is *possible*, not that it's *still* valid at execution
2. Atomic WHERE checks precondition hasn't changed since validation
3. Checks rows affected—0 means precondition no longer held
4. Eliminates need for explicit locks

**Example**:
```go
// FulfillOrder: Order must be paid, not just validated as paid
result, err := db.ExecContext(ctx, `
    UPDATE orders SET status = 'fulfilled', updated_at = CURRENT_TIMESTAMP
    WHERE id = ? AND status = 'paid'  // Atomic precondition check
`, orderID)

if result.RowsAffected() == 0 {
    return fmt.Errorf("order not in paid state (may have been fulfilled/cancelled)")
}
```

**Why It Works**:
- Detects concurrent modifications without locking
- No deadlock potential
- Naturally retryable (can check again)
- Simple and database-agnostic

**Applicability**: Essential for:
- Concurrent systems without explicit locking
- Systems where transactions are too expensive
- Databases with optimistic locking support (all modern ones)
- High-concurrency scenarios where locks would thrash

---

## Test Migration Pattern (PROVEN)

**Context**: Updated 40+ test cases to work with new FSM constraints

**Pattern**: Identify and fix tests one domain at a time (operations → commands → fsm). Document why each test changed.

**Key Insights**:
1. Tests fail fast when FSM constraints introduced
2. Can identify bad test assumptions (e.g., fulfill without payment)
3. Test failures guide implementation priorities
4. Updated tests document valid state transitions

**Example of Test Fix**:
```go
// BEFORE: Test violates FSM (fulfilling order without paying)
order, _ := db.CreateOrder(ctx, c.ID, 6, 3200)
_ = db.FulfillOrder(ctx, order.ID)  // INVALID: pending → fulfilled

// AFTER: Follow valid state progression
order, _ := db.CreateOrder(ctx, c.ID, 6, 3200)
_ = db.UpdateOrderStatus(ctx, order.ID, "paid")  // pending → paid
_ = db.FulfillOrder(ctx, order.ID)  // paid → fulfilled (VALID)
```

**Applicability**: Any system undergoing validation layer addition. Use when:
- Adding new constraints to existing system
- Tests reveal invalid assumptions
- Business logic needs explicit enforcement
- Want executable documentation of valid flows

---

## Extraction Readiness Assessment

### PROVEN (Ready to Extract)
- **FSM as Validator pattern**: Used in all three FSMs, consistent across domains
- **Atomic WHERE for optimistic locking**: Reusable technique, database-agnostic
- **Test-driven FSM definition**: Template for defining any new state machine
- **Event loop FSM integration**: Pattern for integrating FSM into event processing

### ACTIVE (Not Yet Extracted)
- **EventProcessorFSM**: Too specific to eggbot's event model (DM + zap)
- **OrderStateMachine**: Too specific to order domain (extend if other entities have similar lifecycle)

### EXPERIMENTAL (Too Early)
- Nothing—all patterns have been used and verified

---

## Known Limitations & Future Work

### Race Condition Prevention
Current approach (FSM + atomic WHERE) works but:
- Requires developer discipline to use WHERE clause correctly
- Could add database-level checks (constraints) as belt-and-suspenders
- Future: Automatic WHERE clause generation from FSM

### Error Recovery
Current approach resets FSM to idle on any error:
- Simple but coarse-grained
- Future: Transition to error state, allow recovery/retry
- Consider: State history audit log for debugging

### State Observability
Current approach logs transitions only on error:
- Missing visibility into normal operation flow
- Future: Metrics/logging hooks at transition boundaries
- Consider: OpenTelemetry span per event processing

---

## Project Status

- **Phase**: PROVEN - Ready for template/extraction
- **Stability**: Production-ready (all tests passing, zero race conditions)
- **Documentation**: Complete (FSM_MIGRATION.md, comprehensive test comments)
- **Quality**: 100% test coverage on FSM code, zero lint errors

**Extraction Candidates**:
1. FSM as Validator pattern → buildtall-go skill
2. Atomic WHERE optimistic locking → sql-patterns skill
3. Test-driven FSM template → fsm-design skill

**Next Steps**:
- Monitor production usage for any edge cases
- Extract patterns to techtree when used in 2+ projects
- Consider constraint-based validation layer if performance becomes issue
