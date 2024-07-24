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

type RuntimeApplication func(ctx context.Context, shutdown chan struct{}) (<-chan error, error)

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
	lastExecTime    atomic.Value // Use atomic.Value for thread-safe access
	Priority        int
	DynamicInterval func(elapsedTime time.Duration) time.Duration
}

type Clock struct {
	name     string
	interval time.Duration
	ticker   *time.Ticker
	stopCh   chan struct{}
	subs     sync.Map
	ticking  bool
	started  bool
	mu       sync.Mutex
}

func NewClock(opts ...ClockOption) *Clock {
	c := &Clock{
		interval: 100 * time.Millisecond,
		stopCh:   make(chan struct{}),
		ticking:  false,
		started:  false,
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
	c.mu.Lock()
	if c.started {
		c.mu.Unlock()
		return
	}
	c.started = true
	c.ticking = true
	c.mu.Unlock()
	go c.dispatchTicks()
}

func (c *Clock) Stop() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.ticking {
		return
	}
	close(c.stopCh)
	c.ticking = false
	c.started = false
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
	apps map[Key]struct {
		app        RuntimeApplication
		cancelFunc context.CancelFunc
		shutdownCh chan struct{}
	}
	ctx           context.Context
	cancel        context.CancelFunc
	errChan       chan error
	mu            sync.RWMutex
	shutdownTimer time.Duration
}

func NewAppManager[Key RuntimeKey](shutdownTimer time.Duration) *AppManager[Key] {
	ctx, cancel := context.WithCancel(context.Background())
	return &AppManager[Key]{
		apps: make(map[Key]struct {
			app        RuntimeApplication
			cancelFunc context.CancelFunc
			shutdownCh chan struct{}
		}),
		ctx:           ctx,
		cancel:        cancel,
		errChan:       make(chan error),
		shutdownTimer: shutdownTimer,
	}
}

func (am *AppManager[Key]) AddApplication(name Key, app RuntimeApplication) error {
	am.mu.Lock()
	defer am.mu.Unlock()

	if _, exists := am.apps[name]; exists {
		return fmt.Errorf("application with name %v already exists", name)
	}

	appCtx, cancelFunc := context.WithCancel(am.ctx)
	shutdownCh := make(chan struct{})

	am.apps[name] = struct {
		app        RuntimeApplication
		cancelFunc context.CancelFunc
		shutdownCh chan struct{}
	}{app, cancelFunc, shutdownCh}

	go func() {
		appErrChan, err := app(appCtx, shutdownCh)
		if err != nil {
			am.errChan <- fmt.Errorf("error starting application %v: %w", name, err)
			return
		}
		for err := range appErrChan {
			am.errChan <- fmt.Errorf("error from application %v: %w", name, err)
		}
	}()

	return nil
}

func (am *AppManager[Key]) ShutdownApplication(name Key) error {
	am.mu.Lock()
	defer am.mu.Unlock()

	app, exists := am.apps[name]
	if !exists {
		return fmt.Errorf("application with name %v not found", name)
	}

	app.cancelFunc()
	close(app.shutdownCh)

	delete(am.apps, name)

	slog.Info("Application shut down", "name", name)
	return nil
}

func (am *AppManager[Key]) GracefulShutdown() error {
	ctx, cancel := context.WithTimeout(context.Background(), am.shutdownTimer)
	defer cancel()

	var wg sync.WaitGroup
	am.mu.RLock()
	for name, app := range am.apps {
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
		}(name, app)
	}
	am.mu.RUnlock()

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
		am.cancel() // Cancel the main context after graceful shutdown attempt
	}()

	return shutdownChan, am.errChan
}

func createMiddleware(interval time.Duration, mode ExecutionMode, app RuntimeApplication) RuntimeApplication {
	return func(ctx context.Context, shutdown chan struct{}) (<-chan error, error) {
		clock := NewClock(WithInterval(interval))
		errChan := make(chan error)

		wrapper := &tickerWrapper{
			app:      app,
			ctx:      ctx,
			shutdown: shutdown,
			errChan:  errChan,
		}

		clock.Add(wrapper, mode)
		clock.Start()

		go func() {
			defer close(errChan)
			for {
				select {
				case <-ctx.Done():
					clock.Stop()
					return
				case <-shutdown:
					clock.Stop()
					return
				case err := <-wrapper.errChan:
					if err != nil {
						errChan <- err
						clock.Stop()
						close(shutdown)
						return
					}
				}
			}
		}()

		select {
		case <-ctx.Done():
		case <-shutdown:
		}

		return errChan, nil
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

type tickerWrapper struct {
	app      RuntimeApplication
	ctx      context.Context
	shutdown chan struct{}
	errChan  chan error
	running  atomic.Bool
}

func (tw *tickerWrapper) Tick() {
	if !tw.running.CompareAndSwap(false, true) {
		return // Already running
	}
	defer tw.running.Store(false)

	appErrChan, err := tw.app(tw.ctx, tw.shutdown)
	if err != nil {
		select {
		case tw.errChan <- err:
		case <-tw.ctx.Done():
		}
		return
	}

	select {
	case err := <-appErrChan:
		if err != nil {
			select {
			case tw.errChan <- err:
			case <-tw.ctx.Done():
			}
		}
	case <-tw.ctx.Done():
	default:
		// For BestEffort, we don't wait for the app to complete
	}
}
