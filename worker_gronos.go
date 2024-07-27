package gronos

import (
	"context"
	"time"
)

// TickingApplication is a function type representing an application that performs periodic tasks.
type TickingApplication func(context.Context) error

type tickerWrapper struct {
	clock *Clock
	ctx   context.Context
	app   TickingApplication
	cerr  chan error
}

func (t tickerWrapper) Tick() {
	if err := t.app(t.ctx); err != nil {
		t.clock.Stop()
		t.cerr <- err
	}
}

// Worker creates a RuntimeApplication that executes a TickingApplication at specified intervals.
// It takes an interval duration, execution mode, and a TickingApplication as parameters.
//
// Example usage:
//
//	worker := gronos.Worker(time.Second, gronos.NonBlocking, func(ctx context.Context) error {
//		// Periodic task logic here
//		return nil
//	})
//	g.Add("periodicTask", worker)
func Worker(interval time.Duration, mode ExecutionMode, app TickingApplication) RuntimeApplication {
	return func(ctx context.Context, shutdown <-chan struct{}) error {
		w := &tickerWrapper{
			clock: NewClock(WithInterval(interval)),
			ctx:   ctx,
			app:   app,
			cerr:  make(chan error, 1),
		}
		w.clock.Add(w, mode)
		w.clock.Start()

		select {
		case <-ctx.Done():
			w.clock.Stop()
			return ctx.Err()
		case <-shutdown:
			w.clock.Stop()
			return nil
		case err := <-w.cerr:
			w.clock.Stop()
			return err
		}
	}
}
