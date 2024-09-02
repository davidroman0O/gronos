package gronos

import (
	"context"
	"time"

	"github.com/charmbracelet/log"
)

// TickingRuntime is a function type representing an application that performs periodic tasks.
type TickingRuntime func(context.Context) error

type tickerWrapper struct {
	clock     *Clock
	ctx       context.Context
	app       TickingRuntime
	cerr      chan error
	finalTick bool
}

func (t tickerWrapper) Tick() {
	if err := t.app(t.ctx); err != nil {
		t.clock.Stop()
		t.cerr <- err
	}
}

type tickerOption func(*tickerWrapper)

func WithFinalTick() tickerOption {
	return func(t *tickerWrapper) {
		t.finalTick = true
	}
}

// Worker creates a RuntimeApplication that executes a TickingRuntime at specified intervals.
// It takes an interval duration, execution mode, and a TickingRuntime as parameters.
//
// Example usage:
//
//	worker := gronos.Worker(time.Second, gronos.NonBlocking, func(ctx context.Context) error {
//		// Periodic task logic here
//		return nil
//	})
//	g.Add("periodicTask", worker)
func Worker(interval time.Duration, mode ExecutionMode, app TickingRuntime, opts ...tickerOption) LifecyleFunc {
	log.Debug("[Worker] Creating worker")
	return func(ctx context.Context, shutdown <-chan struct{}) error {
		log.Debug("[Worker] Starting worker")
		w := &tickerWrapper{
			clock: NewClock(WithInterval(interval)),
			ctx:   ctx,
			app:   app,
			cerr:  make(chan error, 1),
		}

		for _, opt := range opts {
			opt(w)
		}

		w.clock.Add(w, mode)
		w.clock.Start()

		if w.finalTick {
			defer func() {
				w.Tick() // Run once before returning
			}()
		}

		select {
		case <-ctx.Done():
			log.Debug("[Worker] Context done")
			w.clock.Stop()
			return nil
		case <-shutdown:
			log.Debug("[Worker] Shutdown")
			w.clock.Stop()
			return nil
		case err := <-w.cerr:
			log.Debug("[Worker] Error")
			w.clock.Stop()
			return err
		}
	}
}
