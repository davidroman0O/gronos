package gronos

import (
	"context"
	"log/slog"
	"time"
)

type tickerWrapper struct {
	app      RuntimeApplication
	ctx      context.Context
	shutdown chan struct{}
	errCh    chan error
	cancel   context.CancelFunc
}

func (tw *tickerWrapper) Tick() {
	err := tw.app(tw.ctx, tw.shutdown)
	if err != nil {
		slog.Error("Application error", "error", err)
		select {
		case tw.errCh <- err:
		default:
		}
		tw.cancel()
	}
}

func NonBlockingMiddleware(interval time.Duration, app RuntimeApplication) RuntimeApplication {
	return createMiddleware(interval, NonBlocking, app)
}

func ManagedTimelineMiddleware(interval time.Duration, app RuntimeApplication) RuntimeApplication {
	return createMiddleware(interval, ManagedTimeline, app)
}

func BestEffortMiddleware(interval time.Duration, app RuntimeApplication) RuntimeApplication {
	return createMiddleware(interval, BestEffort, app)
}

func createMiddleware(interval time.Duration, mode ExecutionMode, app RuntimeApplication) RuntimeApplication {
	return func(ctx context.Context, shutdown chan struct{}) error {
		ctx, cancel := context.WithCancel(ctx)
		clock := NewClock(WithInterval(interval))
		wrapper := &tickerWrapper{
			app:      app,
			ctx:      ctx,
			shutdown: shutdown,
			errCh:    make(chan error, 1),
			cancel:   cancel,
		}

		clock.Add(wrapper, mode)
		clock.Start()
		defer clock.Stop()

		select {
		case err := <-wrapper.errCh:
			return err
		case <-ctx.Done():
			return ctx.Err()
		case <-shutdown:
			return nil
		}
	}
}
