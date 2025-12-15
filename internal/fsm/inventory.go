package fsm

import (
	"sync"

	"github.com/looplab/fsm"
)

type InventoryStateMachine struct {
	fsm *fsm.FSM
	mu  sync.Mutex
}

func NewInventoryStateMachine() *InventoryStateMachine {
	ism := &InventoryStateMachine{}
	ism.fsm = fsm.NewFSM(
		InventoryStateAvailable,
		fsm.Events{
			{Name: InventoryEventReserve, Src: []string{InventoryStateAvailable}, Dst: InventoryStateReserved},
			{Name: InventoryEventConsume, Src: []string{InventoryStateReserved}, Dst: InventoryStateConsumed},
			{Name: InventoryEventRestore, Src: []string{InventoryStateReserved}, Dst: InventoryStateAvailable},
		},
		fsm.Callbacks{},
	)
	return ism
}

func (ism *InventoryStateMachine) CanOperation(orderState, operation string) bool {
	inventoryState := ism.orderStateToInventoryState(orderState)
	ism.mu.Lock()
	defer ism.mu.Unlock()
	ism.fsm.SetState(inventoryState)
	return ism.fsm.Can(operation)
}

func (ism *InventoryStateMachine) CanReserve() bool {
	ism.mu.Lock()
	defer ism.mu.Unlock()
	ism.fsm.SetState(InventoryStateAvailable)
	return ism.fsm.Can(InventoryEventReserve)
}

func (ism *InventoryStateMachine) CanRestore(orderState string) bool {
	inventoryState := ism.orderStateToInventoryState(orderState)
	ism.mu.Lock()
	defer ism.mu.Unlock()
	ism.fsm.SetState(inventoryState)
	return ism.fsm.Can(InventoryEventRestore)
}

func (ism *InventoryStateMachine) CanConsume(orderState string) bool {
	inventoryState := ism.orderStateToInventoryState(orderState)
	ism.mu.Lock()
	defer ism.mu.Unlock()
	ism.fsm.SetState(inventoryState)
	return ism.fsm.Can(InventoryEventConsume)
}

func (ism *InventoryStateMachine) orderStateToInventoryState(orderState string) string {
	switch orderState {
	case OrderStatePending, OrderStatePaid:
		return InventoryStateReserved
	case OrderStateFulfilled:
		return InventoryStateConsumed
	case OrderStateCancelled:
		return InventoryStateAvailable
	default:
		return InventoryStateAvailable
	}
}
