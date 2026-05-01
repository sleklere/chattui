package bus

import (
	"context"
	"log"

	"github.com/sleklere/realtime-chat/cmd/server/internal/event"
)

const QUEUE_CHAN_SIZE = 512

// Bus is the in-process event bus. Publishers call Publish; consumers register handlers via Subscribe.
type Bus interface {
	Publish(ctx context.Context, event event.Event)
	Subscribe(kind string, fn func(ctx context.Context, event event.Event) error)
}

type customBus struct {
	queue    chan event.Event
	handlers map[string]func(ctx context.Context, event event.Event) error
}

// NewBus creates and starts an in-process event bus backed by a buffered channel.
// TODO: accept a cancelable ctx from main for graceful shutdown (cancel dispatch loop on SIGTERM).
// Requires wiring a root context.WithCancel in main.go.
func NewBus() Bus {
	b := &customBus{
		queue:    make(chan event.Event, QUEUE_CHAN_SIZE),
		handlers: make(map[string]func(ctx context.Context, event event.Event) error),
	}

	go b.dispatch()

	return b
}

func (b *customBus) Publish(ctx context.Context, event event.Event) {
	b.queue <- event
}

func (b *customBus) Subscribe(kind string, fn func(ctx context.Context, event event.Event) error) {
	b.handlers[kind] = fn
}

func (b *customBus) dispatch() {
	for e := range b.queue {
		fn, ok := b.handlers[e.Kind()]
		if !ok {
			log.Printf("bus: no handler registered for event kind %q", e.Kind())
			continue
		}
		// TODO: implement semaphore/worker pool to bound concurrency
		go fn(context.Background(), e)
	}
}
