package fsm

import (
	"sync"
	"testing"
)

func TestInventoryStateMachine_CanReserve(t *testing.T) {
	ism := NewInventoryStateMachine()

	if !ism.CanReserve() {
		t.Error("should be able to reserve from available state")
	}
}

func TestInventoryStateMachine_CanRestore(t *testing.T) {
	ism := NewInventoryStateMachine()

	tests := []struct {
		orderState string
		want       bool
	}{
		{OrderStatePending, true},
		{OrderStatePaid, true},
		{OrderStateFulfilled, false},
		{OrderStateCancelled, false},
	}

	for _, tt := range tests {
		t.Run(tt.orderState, func(t *testing.T) {
			got := ism.CanRestore(tt.orderState)
			if got != tt.want {
				t.Errorf("CanRestore(%s) = %v, want %v", tt.orderState, got, tt.want)
			}
		})
	}
}

func TestInventoryStateMachine_CanConsume(t *testing.T) {
	ism := NewInventoryStateMachine()

	tests := []struct {
		orderState string
		want       bool
	}{
		{OrderStatePending, true},
		{OrderStatePaid, true},
		{OrderStateFulfilled, false},
		{OrderStateCancelled, false},
	}

	for _, tt := range tests {
		t.Run(tt.orderState, func(t *testing.T) {
			got := ism.CanConsume(tt.orderState)
			if got != tt.want {
				t.Errorf("CanConsume(%s) = %v, want %v", tt.orderState, got, tt.want)
			}
		})
	}
}

func TestInventoryStateMachine_CanOperation(t *testing.T) {
	ism := NewInventoryStateMachine()

	tests := []struct {
		orderState string
		operation  string
		want       bool
	}{
		{"", InventoryEventReserve, true},
		{OrderStatePending, InventoryEventRestore, true},
		{OrderStatePending, InventoryEventConsume, true},
		{OrderStatePaid, InventoryEventRestore, true},
		{OrderStatePaid, InventoryEventConsume, true},
		{OrderStateFulfilled, InventoryEventRestore, false},
		{OrderStateFulfilled, InventoryEventConsume, false},
		{OrderStateCancelled, InventoryEventRestore, false},
		{OrderStateCancelled, InventoryEventConsume, false},
		{OrderStateCancelled, InventoryEventReserve, true},
	}

	for _, tt := range tests {
		name := tt.orderState + "_" + tt.operation
		if tt.orderState == "" {
			name = "new_" + tt.operation
		}
		t.Run(name, func(t *testing.T) {
			got := ism.CanOperation(tt.orderState, tt.operation)
			if got != tt.want {
				t.Errorf("CanOperation(%s, %s) = %v, want %v", tt.orderState, tt.operation, got, tt.want)
			}
		})
	}
}

func TestInventoryStateMachine_OrderStateMapping(t *testing.T) {
	ism := NewInventoryStateMachine()

	tests := []struct {
		orderState     string
		inventoryState string
	}{
		{OrderStatePending, InventoryStateReserved},
		{OrderStatePaid, InventoryStateReserved},
		{OrderStateFulfilled, InventoryStateConsumed},
		{OrderStateCancelled, InventoryStateAvailable},
		{"", InventoryStateAvailable},
		{"unknown", InventoryStateAvailable},
	}

	for _, tt := range tests {
		t.Run(tt.orderState, func(t *testing.T) {
			got := ism.orderStateToInventoryState(tt.orderState)
			if got != tt.inventoryState {
				t.Errorf("orderStateToInventoryState(%s) = %s, want %s", tt.orderState, got, tt.inventoryState)
			}
		})
	}
}

func TestInventoryStateMachine_ConcurrentAccess(t *testing.T) {
	ism := NewInventoryStateMachine()
	var wg sync.WaitGroup

	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()

			ism.CanReserve()
			ism.CanRestore(OrderStatePending)
			ism.CanConsume(OrderStatePaid)
			ism.CanOperation(OrderStatePending, InventoryEventRestore)
		}()
	}

	wg.Wait()
}
