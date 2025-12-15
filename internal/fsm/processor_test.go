package fsm

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/looplab/fsm"
)

func TestEventProcessorFSM_DMFlow(t *testing.T) {
	ep := NewEventProcessorFSM()
	ctx := context.Background()

	if ep.Current() != ProcessorStateIdle {
		t.Errorf("initial state should be idle, got %s", ep.Current())
	}

	if err := ep.Event(ctx, ProcessorEventDMReceived); err != nil {
		t.Errorf("dm_received error: %v", err)
	}
	if ep.Current() != ProcessorStateProcessingDM {
		t.Errorf("should be processing_dm, got %s", ep.Current())
	}

	if err := ep.Event(ctx, ProcessorEventCommandProcessed); err != nil {
		t.Errorf("command_processed error: %v", err)
	}
	if ep.Current() != ProcessorStateSendingResponse {
		t.Errorf("should be sending_response, got %s", ep.Current())
	}

	if err := ep.Event(ctx, ProcessorEventResponseSent); err != nil {
		t.Errorf("response_sent error: %v", err)
	}
	if ep.Current() != ProcessorStateIdle {
		t.Errorf("should be idle, got %s", ep.Current())
	}
}

func TestEventProcessorFSM_ZapFlow(t *testing.T) {
	ep := NewEventProcessorFSM()
	ctx := context.Background()

	if err := ep.Event(ctx, ProcessorEventZapReceived); err != nil {
		t.Errorf("zap_received error: %v", err)
	}
	if ep.Current() != ProcessorStateProcessingZap {
		t.Errorf("should be processing_zap, got %s", ep.Current())
	}

	if err := ep.Event(ctx, ProcessorEventResponseSent); err != nil {
		t.Errorf("response_sent error: %v", err)
	}
	if ep.Current() != ProcessorStateIdle {
		t.Errorf("should be idle, got %s", ep.Current())
	}
}

func TestEventProcessorFSM_ErrorRecovery(t *testing.T) {
	tests := []struct {
		name       string
		setupState string
		setupEvent string
	}{
		{
			name:       "error from processing_dm",
			setupState: ProcessorStateProcessingDM,
			setupEvent: ProcessorEventDMReceived,
		},
		{
			name:       "error from processing_zap",
			setupState: ProcessorStateProcessingZap,
			setupEvent: ProcessorEventZapReceived,
		},
		{
			name:       "error from sending_response",
			setupState: ProcessorStateSendingResponse,
			setupEvent: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ep := NewEventProcessorFSM()
			ctx := context.Background()

			if tt.setupEvent != "" {
				_ = ep.Event(ctx, tt.setupEvent)
			} else {
				_ = ep.Event(ctx, ProcessorEventDMReceived)
				_ = ep.Event(ctx, ProcessorEventCommandProcessed)
			}

			if err := ep.Event(ctx, ProcessorEventError); err != nil {
				t.Errorf("error event should succeed: %v", err)
			}
			if ep.Current() != ProcessorStateIdle {
				t.Errorf("should recover to idle, got %s", ep.Current())
			}
		})
	}
}

func TestEventProcessorFSM_InvalidTransitions(t *testing.T) {
	tests := []struct {
		name  string
		setup func(*EventProcessorFSM, context.Context)
		event string
	}{
		{
			name:  "dm_received when not idle",
			setup: func(ep *EventProcessorFSM, ctx context.Context) { _ = ep.Event(ctx, ProcessorEventDMReceived) },
			event: ProcessorEventDMReceived,
		},
		{
			name:  "zap_received when not idle",
			setup: func(ep *EventProcessorFSM, ctx context.Context) { _ = ep.Event(ctx, ProcessorEventDMReceived) },
			event: ProcessorEventZapReceived,
		},
		{
			name:  "command_processed from idle",
			setup: func(ep *EventProcessorFSM, ctx context.Context) {},
			event: ProcessorEventCommandProcessed,
		},
		{
			name:  "command_processed from processing_zap",
			setup: func(ep *EventProcessorFSM, ctx context.Context) { _ = ep.Event(ctx, ProcessorEventZapReceived) },
			event: ProcessorEventCommandProcessed,
		},
		{
			name:  "response_sent from idle",
			setup: func(ep *EventProcessorFSM, ctx context.Context) {},
			event: ProcessorEventResponseSent,
		},
		{
			name:  "error from idle",
			setup: func(ep *EventProcessorFSM, ctx context.Context) {},
			event: ProcessorEventError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ep := NewEventProcessorFSM()
			ctx := context.Background()

			tt.setup(ep, ctx)

			err := ep.Event(ctx, tt.event)
			if err == nil {
				t.Errorf("expected error for invalid transition")
			}

			var invalidErr fsm.InvalidEventError
			if !errors.As(err, &invalidErr) {
				t.Errorf("expected InvalidEventError, got %T: %v", err, err)
			}
		})
	}
}

func TestEventProcessorFSM_Can(t *testing.T) {
	ep := NewEventProcessorFSM()
	ctx := context.Background()

	if !ep.Can(ProcessorEventDMReceived) {
		t.Error("should be able to receive DM from idle")
	}
	if !ep.Can(ProcessorEventZapReceived) {
		t.Error("should be able to receive zap from idle")
	}
	if ep.Can(ProcessorEventCommandProcessed) {
		t.Error("should not be able to process command from idle")
	}

	_ = ep.Event(ctx, ProcessorEventDMReceived)

	if ep.Can(ProcessorEventDMReceived) {
		t.Error("should not be able to receive DM while processing")
	}
	if !ep.Can(ProcessorEventCommandProcessed) {
		t.Error("should be able to process command from processing_dm")
	}
	if !ep.Can(ProcessorEventError) {
		t.Error("should be able to error from processing_dm")
	}
}

func TestEventProcessorFSM_Callbacks(t *testing.T) {
	ep := NewEventProcessorFSM()
	ctx := context.Background()

	var enterCount, leaveCount int32

	ep.OnEnter(ProcessorStateProcessingDM, func() {
		atomic.AddInt32(&enterCount, 1)
	})
	ep.OnLeave(ProcessorStateIdle, func() {
		atomic.AddInt32(&leaveCount, 1)
	})

	_ = ep.Event(ctx, ProcessorEventDMReceived)

	if atomic.LoadInt32(&enterCount) != 1 {
		t.Errorf("enter callback should have been called once, got %d", enterCount)
	}
	if atomic.LoadInt32(&leaveCount) != 1 {
		t.Errorf("leave callback should have been called once, got %d", leaveCount)
	}
}

func TestEventProcessorFSM_Reset(t *testing.T) {
	ep := NewEventProcessorFSM()
	ctx := context.Background()

	_ = ep.Event(ctx, ProcessorEventDMReceived)
	if ep.Current() != ProcessorStateProcessingDM {
		t.Errorf("should be processing_dm, got %s", ep.Current())
	}

	ep.Reset()
	if ep.Current() != ProcessorStateIdle {
		t.Errorf("should be idle after reset, got %s", ep.Current())
	}
}

func TestEventProcessorFSM_ConcurrentAccess(t *testing.T) {
	ep := NewEventProcessorFSM()
	ctx := context.Background()
	var wg sync.WaitGroup

	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()

			ep.Current()
			ep.Can(ProcessorEventDMReceived)

			ep.Reset()
			_ = ep.Event(ctx, ProcessorEventDMReceived)
			_ = ep.Event(ctx, ProcessorEventError)
		}()
	}

	wg.Wait()
}
