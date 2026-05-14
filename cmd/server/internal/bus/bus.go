// Package bus provides an in-process event bus for decoupled domain event publishing.
package bus

import (
	"context"
	"log/slog"

	"github.com/sleklere/chattui/cmd/server/internal/event"
)

const queueChanSize = 512

// Bus is the in-process event bus. Publishers call Publish; consumers register handlers via Subscribe.
type Bus interface {
	Publish(ctx context.Context, event event.Event)
	Subscribe(kind string, fn func(ctx context.Context, event event.Event) error)
}

type customBus struct {
	queue    chan event.Event
	handlers map[string][]func(ctx context.Context, event event.Event) error
	logger   *slog.Logger
}

// NewBus creates and starts an in-process event bus backed by a buffered channel.
// TODO: accept a cancelable ctx from main for graceful shutdown (cancel dispatch loop on SIGTERM).
// Requires wiring a root context.WithCancel in main.go.
func NewBus(l *slog.Logger) Bus {
	b := &customBus{
		queue:    make(chan event.Event, queueChanSize),
		handlers: make(map[string][]func(ctx context.Context, event event.Event) error),
		logger:   l,
	}

	go b.dispatch()

	return b
}

func (b *customBus) Publish(_ context.Context, event event.Event) {
	b.queue <- event
}

func (b *customBus) Subscribe(kind string, fn func(ctx context.Context, event event.Event) error) {
	b.handlers[kind] = append(b.handlers[kind], fn)
}

func (b *customBus) dispatch() {
	for e := range b.queue {
		handlers, ok := b.handlers[e.Kind()]
		if !ok {
			b.logger.Warn("bus: no handler registered", "kind", e.Kind())
			continue
		}
		for _, h := range handlers {
			// TODO: implement semaphore/worker pool to bound concurrency
			go func() {
				err := h(context.Background(), e)
				if err != nil {
					b.logger.Error("bus: handler error", "kind", e.Kind(), "error", err)
				}
			}()
		}
	}
}
