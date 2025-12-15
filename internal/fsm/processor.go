package fsm

import (
	"context"
	"sync"

	"github.com/looplab/fsm"
)

type EventProcessorFSM struct {
	fsm     *fsm.FSM
	mu      sync.Mutex
	onEnter map[string]func()
	onLeave map[string]func()
}

func NewEventProcessorFSM() *EventProcessorFSM {
	ep := &EventProcessorFSM{
		onEnter: make(map[string]func()),
		onLeave: make(map[string]func()),
	}
	ep.fsm = fsm.NewFSM(
		ProcessorStateIdle,
		fsm.Events{
			{Name: ProcessorEventDMReceived, Src: []string{ProcessorStateIdle}, Dst: ProcessorStateProcessingDM},
			{Name: ProcessorEventZapReceived, Src: []string{ProcessorStateIdle}, Dst: ProcessorStateProcessingZap},
			{Name: ProcessorEventCommandProcessed, Src: []string{ProcessorStateProcessingDM}, Dst: ProcessorStateSendingResponse},
			{Name: ProcessorEventResponseSent, Src: []string{ProcessorStateSendingResponse, ProcessorStateProcessingZap}, Dst: ProcessorStateIdle},
			{Name: ProcessorEventError, Src: []string{ProcessorStateProcessingDM, ProcessorStateProcessingZap, ProcessorStateSendingResponse}, Dst: ProcessorStateIdle},
		},
		fsm.Callbacks{
			"enter_state": func(_ context.Context, e *fsm.Event) {
				if fn, ok := ep.onEnter[e.Dst]; ok {
					fn()
				}
			},
			"leave_state": func(_ context.Context, e *fsm.Event) {
				if fn, ok := ep.onLeave[e.Src]; ok {
					fn()
				}
			},
		},
	)
	return ep
}

func (ep *EventProcessorFSM) Current() string {
	ep.mu.Lock()
	defer ep.mu.Unlock()
	return ep.fsm.Current()
}

func (ep *EventProcessorFSM) Event(ctx context.Context, event string) error {
	ep.mu.Lock()
	defer ep.mu.Unlock()
	return ep.fsm.Event(ctx, event)
}

func (ep *EventProcessorFSM) Can(event string) bool {
	ep.mu.Lock()
	defer ep.mu.Unlock()
	return ep.fsm.Can(event)
}

func (ep *EventProcessorFSM) OnEnter(state string, fn func()) {
	ep.mu.Lock()
	defer ep.mu.Unlock()
	ep.onEnter[state] = fn
}

func (ep *EventProcessorFSM) OnLeave(state string, fn func()) {
	ep.mu.Lock()
	defer ep.mu.Unlock()
	ep.onLeave[state] = fn
}

func (ep *EventProcessorFSM) Reset() {
	ep.mu.Lock()
	defer ep.mu.Unlock()
	ep.fsm.SetState(ProcessorStateIdle)
}
