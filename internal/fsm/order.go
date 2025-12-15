package fsm

import (
	"context"
	"sync"

	"github.com/looplab/fsm"
)

type OrderStateMachine struct {
	fsm *fsm.FSM
	mu  sync.Mutex
}

func NewOrderStateMachine() *OrderStateMachine {
	osm := &OrderStateMachine{}
	osm.fsm = fsm.NewFSM(
		OrderStatePending,
		fsm.Events{
			{Name: OrderEventPay, Src: []string{OrderStatePending}, Dst: OrderStatePaid},
			{Name: OrderEventCancel, Src: []string{OrderStatePending}, Dst: OrderStateCancelled},
			{Name: OrderEventFulfill, Src: []string{OrderStatePaid}, Dst: OrderStateFulfilled},
		},
		fsm.Callbacks{},
	)
	return osm
}

func (osm *OrderStateMachine) CanTransition(currentState, event string) bool {
	osm.mu.Lock()
	defer osm.mu.Unlock()
	osm.fsm.SetState(currentState)
	return osm.fsm.Can(event)
}

func (osm *OrderStateMachine) Transition(ctx context.Context, currentState, event string) (string, error) {
	osm.mu.Lock()
	defer osm.mu.Unlock()
	osm.fsm.SetState(currentState)
	if err := osm.fsm.Event(ctx, event); err != nil {
		return "", err
	}
	return osm.fsm.Current(), nil
}

func (osm *OrderStateMachine) AvailableEvents(currentState string) []string {
	osm.mu.Lock()
	defer osm.mu.Unlock()
	osm.fsm.SetState(currentState)
	return osm.fsm.AvailableTransitions()
}
