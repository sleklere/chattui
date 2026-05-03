package bus

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/sleklere/chattui/cmd/server/internal/event"
)

func noopLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func newTestBus() *customBus {
	return &customBus{
		queue:    make(chan event.Event, 3000),
		handlers: make(map[string]func(context.Context, event.Event) error),
		logger:   noopLogger(),
	}
}

// ---------------------------------------------------------------------------
// Publish
// ---------------------------------------------------------------------------

func TestPublish_AddsToQueue(t *testing.T) {
	cases := []struct {
		name   string
		events int
	}{
		{"one event", 1},
		{"three events", 3},
		{"many events", 2345},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			b := newTestBus()
			for range tc.events {
				b.Publish(context.Background(), event.RoomJoinedEvent{UserID: 1, RoomID: 2})
			}
			if len(b.queue) != tc.events {
				t.Fatalf("expected %d events in queue, got %d", tc.events, len(b.queue))
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Subscribe
// ---------------------------------------------------------------------------

func TestSubscribe_RegistersHandler(t *testing.T) {
	b := newTestBus()

	b.Subscribe("room_join", func(_ context.Context, _ event.Event) error { return nil })

	if _, ok := b.handlers["room_join"]; !ok {
		t.Fatal("expected handler registered for kind room_join")
	}
	if len(b.handlers) != 1 {
		t.Fatalf("expected 1 handler, got %d", len(b.handlers))
	}
}

// ---------------------------------------------------------------------------
// Dispatch
// ---------------------------------------------------------------------------

// dispatch calls the registered handler for the event kind.
func TestDispatch_CallsHandler(t *testing.T) {
	b := newTestBus()
	called := make(chan event.Event, 1)

	b.Subscribe("room_join", func(_ context.Context, e event.Event) error {
		called <- e
		return nil
	})

	go b.dispatch()
	b.Publish(context.Background(), event.RoomJoinedEvent{UserID: 1, RoomID: 2})

	select {
	case e := <-called:
		ev, ok := e.(event.RoomJoinedEvent)
		if !ok {
			t.Fatalf("expected RoomJoinedEvent, got %T", e)
		}
		if ev.UserID != 1 || ev.RoomID != 2 {
			t.Fatalf("unexpected event fields: %+v", ev)
		}
	case <-time.After(time.Second):
		t.Fatal("handler was not called within timeout")
	}
}

// dispatch continues after a handler returns an error.
func TestDispatch_HandlerError_Continues(t *testing.T) {
	b := newTestBus()
	second := make(chan struct{}, 1)

	calls := 0
	b.Subscribe("room_join", func(_ context.Context, _ event.Event) error {
		calls++
		if calls == 1 {
			return fmt.Errorf("handler error")
		}
		second <- struct{}{}
		return nil
	})

	go b.dispatch()

	b.Publish(context.Background(), event.RoomJoinedEvent{UserID: 1, RoomID: 1})
	b.Publish(context.Background(), event.RoomJoinedEvent{UserID: 2, RoomID: 2})

	select {
	case <-second:
		// dispatch kept running after handler error
	case <-time.After(time.Second):
		t.Fatal("dispatch did not continue after handler error")
	}
}

// dispatch continues without panicking when no handler is registered for a kind.
func TestDispatch_NoHandler_Continues(t *testing.T) {
	b := newTestBus()
	called := make(chan struct{}, 1)

	// register a handler for a different kind as a sync probe
	b.Subscribe("room_leave", func(_ context.Context, _ event.Event) error {
		called <- struct{}{}
		return nil
	})

	go b.dispatch()

	// publish an event with no handler — should not block or panic
	b.Publish(context.Background(), event.RoomJoinedEvent{UserID: 1, RoomID: 2})
	// then publish one that has a handler to confirm dispatch kept running
	b.Publish(context.Background(), event.RoomLeftEvent{UserID: 1, RoomID: 2})

	select {
	case <-called:
		// dispatch continued past the unhandled event
	case <-time.After(time.Second):
		t.Fatal("dispatch did not continue after unhandled event")
	}
}
