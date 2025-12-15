package fsm

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/looplab/fsm"
)

func TestOrderStateMachine_ValidTransitions(t *testing.T) {
	tests := []struct {
		name         string
		currentState string
		event        string
		wantState    string
	}{
		{
			name:         "pending to paid via pay event",
			currentState: OrderStatePending,
			event:        OrderEventPay,
			wantState:    OrderStatePaid,
		},
		{
			name:         "pending to cancelled via cancel event",
			currentState: OrderStatePending,
			event:        OrderEventCancel,
			wantState:    OrderStateCancelled,
		},
		{
			name:         "paid to fulfilled via fulfill event",
			currentState: OrderStatePaid,
			event:        OrderEventFulfill,
			wantState:    OrderStateFulfilled,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			osm := NewOrderStateMachine()
			ctx := context.Background()

			newState, err := osm.Transition(ctx, tt.currentState, tt.event)
			if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
			if newState != tt.wantState {
				t.Errorf("got state %q, want %q", newState, tt.wantState)
			}
		})
	}
}

func TestOrderStateMachine_InvalidTransitions(t *testing.T) {
	tests := []struct {
		name         string
		currentState string
		event        string
	}{
		{
			name:         "pending cannot fulfill directly",
			currentState: OrderStatePending,
			event:        OrderEventFulfill,
		},
		{
			name:         "paid cannot cancel",
			currentState: OrderStatePaid,
			event:        OrderEventCancel,
		},
		{
			name:         "paid cannot pay again",
			currentState: OrderStatePaid,
			event:        OrderEventPay,
		},
		{
			name:         "fulfilled is terminal - cannot pay",
			currentState: OrderStateFulfilled,
			event:        OrderEventPay,
		},
		{
			name:         "fulfilled is terminal - cannot cancel",
			currentState: OrderStateFulfilled,
			event:        OrderEventCancel,
		},
		{
			name:         "fulfilled is terminal - cannot fulfill again",
			currentState: OrderStateFulfilled,
			event:        OrderEventFulfill,
		},
		{
			name:         "cancelled is terminal - cannot pay",
			currentState: OrderStateCancelled,
			event:        OrderEventPay,
		},
		{
			name:         "cancelled is terminal - cannot cancel again",
			currentState: OrderStateCancelled,
			event:        OrderEventCancel,
		},
		{
			name:         "cancelled is terminal - cannot fulfill",
			currentState: OrderStateCancelled,
			event:        OrderEventFulfill,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			osm := NewOrderStateMachine()
			ctx := context.Background()

			_, err := osm.Transition(ctx, tt.currentState, tt.event)
			if err == nil {
				t.Errorf("expected error for invalid transition %s + %s", tt.currentState, tt.event)
			}

			var invalidErr fsm.InvalidEventError
			if !errors.As(err, &invalidErr) {
				t.Errorf("expected InvalidEventError, got %T: %v", err, err)
			}
		})
	}
}

func TestOrderStateMachine_CanTransition(t *testing.T) {
	osm := NewOrderStateMachine()

	tests := []struct {
		currentState string
		event        string
		want         bool
	}{
		{OrderStatePending, OrderEventPay, true},
		{OrderStatePending, OrderEventCancel, true},
		{OrderStatePending, OrderEventFulfill, false},
		{OrderStatePaid, OrderEventFulfill, true},
		{OrderStatePaid, OrderEventPay, false},
		{OrderStatePaid, OrderEventCancel, false},
		{OrderStateFulfilled, OrderEventPay, false},
		{OrderStateFulfilled, OrderEventCancel, false},
		{OrderStateFulfilled, OrderEventFulfill, false},
		{OrderStateCancelled, OrderEventPay, false},
		{OrderStateCancelled, OrderEventCancel, false},
		{OrderStateCancelled, OrderEventFulfill, false},
	}

	for _, tt := range tests {
		name := tt.currentState + "_" + tt.event
		t.Run(name, func(t *testing.T) {
			got := osm.CanTransition(tt.currentState, tt.event)
			if got != tt.want {
				t.Errorf("CanTransition(%s, %s) = %v, want %v", tt.currentState, tt.event, got, tt.want)
			}
		})
	}
}

func TestOrderStateMachine_AvailableEvents(t *testing.T) {
	osm := NewOrderStateMachine()

	tests := []struct {
		currentState string
		wantEvents   []string
	}{
		{OrderStatePending, []string{OrderEventPay, OrderEventCancel}},
		{OrderStatePaid, []string{OrderEventFulfill}},
		{OrderStateFulfilled, []string{}},
		{OrderStateCancelled, []string{}},
	}

	for _, tt := range tests {
		t.Run(tt.currentState, func(t *testing.T) {
			got := osm.AvailableEvents(tt.currentState)

			if len(got) != len(tt.wantEvents) {
				t.Errorf("got %d events, want %d", len(got), len(tt.wantEvents))
				return
			}

			gotSet := make(map[string]bool)
			for _, e := range got {
				gotSet[e] = true
			}

			for _, want := range tt.wantEvents {
				if !gotSet[want] {
					t.Errorf("missing expected event %q in %v", want, got)
				}
			}
		})
	}
}

func TestOrderStateMachine_ConcurrentAccess(t *testing.T) {
	osm := NewOrderStateMachine()
	ctx := context.Background()
	var wg sync.WaitGroup

	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()

			osm.CanTransition(OrderStatePending, OrderEventPay)
			osm.CanTransition(OrderStatePaid, OrderEventFulfill)
			osm.AvailableEvents(OrderStatePending)

			_, _ = osm.Transition(ctx, OrderStatePending, OrderEventPay)
			_, _ = osm.Transition(ctx, OrderStatePaid, OrderEventFulfill)
		}()
	}

	wg.Wait()
}

func TestOrderStateMachine_UnknownEvent(t *testing.T) {
	osm := NewOrderStateMachine()
	ctx := context.Background()

	_, err := osm.Transition(ctx, OrderStatePending, "unknown_event")
	if err == nil {
		t.Error("expected error for unknown event")
	}

	var unknownErr fsm.UnknownEventError
	if !errors.As(err, &unknownErr) {
		t.Errorf("expected UnknownEventError, got %T: %v", err, err)
	}
}
