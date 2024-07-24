package gronos

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"
)

type RuntimeKey comparable

type RuntimeApplication func(ctx context.Context, shutdown chan struct{}) error

type Ticker interface {
	Tick()
}

type ExecutionMode int

const (
	NonBlocking ExecutionMode = iota
	ManagedTimeline
	BestEffort
)

type TickerSubscriber struct {
	Ticker          Ticker
	Mode            ExecutionMode
	lastExecTime    atomic.Value
	Priority        int
	DynamicInterval func(elapsedTime time.Duration) time.Duration
}

type Clock struct {
	name     string
	interval time.Duration
	ticker   *time.Ticker
	stopCh   chan struct{}
	subs     sync.Map
	ticking  atomic.Bool
	started  atomic.Bool
	mu       sync.Mutex
}

func NewClock(opts ...ClockOption) *Clock {
	c := &Clock{
		interval: 100 * time.Millisecond,
		stopCh:   make(chan struct{}),
	}
	for _, opt := range opts {
		opt(c)
	}
	c.ticker = time.NewTicker(c.interval)
	return c
}

type ClockOption func(*Clock)

func WithName(name string) ClockOption {
	return func(c *Clock) {
		c.name = name
	}
}

func WithInterval(interval time.Duration) ClockOption {
	return func(c *Clock) {
		c.interval = interval
	}
}

func (c *Clock) Add(ticker Ticker, mode ExecutionMode) {
	sub := &TickerSubscriber{
		Ticker:   ticker,
		Mode:     mode,
		Priority: 0,
		DynamicInterval: func(elapsedTime time.Duration) time.Duration {
			return c.interval
		},
	}
	sub.lastExecTime.Store(time.Now())
	c.subs.Store(ticker, sub)
}

func (c *Clock) Start() {
	if !c.started.CompareAndSwap(false, true) {
		return
	}
	c.ticking.Store(true)
	go c.dispatchTicks()
}

func (c *Clock) Stop() {
	if !c.ticking.CompareAndSwap(true, false) {
		return
	}
	close(c.stopCh)
	c.started.Store(false)
}

func (c *Clock) dispatchTicks() {
	for {
		select {
		case <-c.ticker.C:
			now := time.Now()
			c.subs.Range(func(key, value interface{}) bool {
				sub := value.(*TickerSubscriber)
				lastExecTime := sub.lastExecTime.Load().(time.Time)
				if now.Sub(lastExecTime) >= sub.DynamicInterval(now.Sub(lastExecTime)) {
					switch sub.Mode {
					case NonBlocking:
						go c.executeTick(key, sub, now)
					case ManagedTimeline:
						c.executeTick(key, sub, now)
					case BestEffort:
						go c.executeTick(key, sub, now)
					}
				}
				return true
			})
		case <-c.stopCh:
			c.ticker.Stop()
			return
		}
	}
}

func (c *Clock) executeTick(key interface{}, sub *TickerSubscriber, now time.Time) {
	sub.Ticker.Tick()
	sub.lastExecTime.Store(now)
	c.subs.Store(key, sub)
}

type AppManager[Key RuntimeKey] struct {
	apps          sync.Map
	ctx           context.Context
	cancel        context.CancelFunc
	errChan       chan error
	shutdownTimer time.Duration
}

func NewAppManager[Key RuntimeKey](shutdownTimer time.Duration) *AppManager[Key] {
	ctx, cancel := context.WithCancel(context.Background())
	return &AppManager[Key]{
		ctx:           ctx,
		cancel:        cancel,
		errChan:       make(chan error, 1),
		shutdownTimer: shutdownTimer,
	}
}

func (am *AppManager[Key]) AddApplication(name Key, app RuntimeApplication) error {
	appCtx, cancelFunc := context.WithCancel(am.ctx)
	shutdownCh := make(chan struct{})

	_, loaded := am.apps.LoadOrStore(name, struct {
		app        RuntimeApplication
		cancelFunc context.CancelFunc
		shutdownCh chan struct{}
	}{app, cancelFunc, shutdownCh})

	if loaded {
		cancelFunc()
		return fmt.Errorf("application with name %v already exists", name)
	}

	go func() {
		defer am.ShutdownApplication(name)
		if err := app(appCtx, shutdownCh); err != nil {
			select {
			case am.errChan <- fmt.Errorf("error from application %v: %w", name, err):
			default:
				// Error channel is full or closed, log the error
				slog.Error("Failed to send error to channel", "name", name, "error", err)
			}
		}
	}()

	return nil
}

func (am *AppManager[Key]) ShutdownApplication(name Key) error {
	value, loaded := am.apps.LoadAndDelete(name)
	if !loaded {
		return fmt.Errorf("application with name %v not found", name)
	}

	app := value.(struct {
		app        RuntimeApplication
		cancelFunc context.CancelFunc
		shutdownCh chan struct{}
	})

	app.cancelFunc()

	select {
	case <-app.shutdownCh:
		// Channel is already closed, do nothing
	default:
		close(app.shutdownCh)
	}

	slog.Info("Application shut down", "name", name)
	return nil
}

func (am *AppManager[Key]) GracefulShutdown() error {
	ctx, cancel := context.WithTimeout(context.Background(), am.shutdownTimer)
	defer cancel()

	var wg sync.WaitGroup
	am.apps.Range(func(key, value interface{}) bool {
		wg.Add(1)
		go func(name Key, app struct {
			app        RuntimeApplication
			cancelFunc context.CancelFunc
			shutdownCh chan struct{}
		}) {
			defer wg.Done()
			app.cancelFunc()
			close(app.shutdownCh)
			slog.Info("Shutting down application", "name", name)
		}(key.(Key), value.(struct {
			app        RuntimeApplication
			cancelFunc context.CancelFunc
			shutdownCh chan struct{}
		}))
		return true
	})

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-done:
		return nil
	}
}

func (am *AppManager[Key]) Run(apps map[Key]RuntimeApplication) (chan struct{}, <-chan error) {
	for name, app := range apps {
		if err := am.AddApplication(name, app); err != nil {
			slog.Error("Failed to add application", "name", name, "error", err)
		}
	}

	shutdownChan := make(chan struct{})
	go func() {
		<-shutdownChan
		if err := am.GracefulShutdown(); err != nil {
			slog.Error("Error during graceful shutdown", "error", err)
		}
		am.cancel()
		close(am.errChan)
	}()

	return shutdownChan, am.errChan
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

		// Wait for context cancellation, shutdown signal, or an application error
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
