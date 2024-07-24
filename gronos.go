package gronos

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"
)

// RuntimeKey is a type constraint for keys used in runtime management.
type RuntimeKey comparable

// RuntimeApplication defines a function type for applications managed by AppManager.
// The application receives a context and a shutdown channel and returns an error.
type RuntimeApplication func(ctx context.Context, shutdown chan struct{}) error

// Ticker is an interface for objects that need to be periodically ticked.
type Ticker interface {
	Tick()
}

// ExecutionMode represents the mode of execution for TickerSubscribers.
type ExecutionMode int

const (
	// NonBlocking mode allows tickers to run in separate goroutines.
	NonBlocking ExecutionMode = iota
	// ManagedTimeline mode runs tickers in the main dispatch loop, ensuring order.
	ManagedTimeline
	// BestEffort mode tries to run tickers as often as possible, without strict timing guarantees.
	BestEffort
)

// TickerSubscriber is a structure that holds information about a subscribed Ticker.
type TickerSubscriber struct {
	Ticker          Ticker
	Mode            ExecutionMode
	lastExecTime    atomic.Value
	Priority        int
	DynamicInterval func(elapsedTime time.Duration) time.Duration
}

// Clock is a structure for managing and dispatching periodic ticks to subscribers.
// It uses a sync.Map to store TickerSubscribers and dispatches ticks based on a defined interval.
type Clock struct {
	name     string
	interval time.Duration
	ticker   *time.Ticker
	stopCh   chan struct{}
	subs     sync.Map
	ticking  atomic.Bool
	started  atomic.Bool
}

// NewClock creates a new Clock instance with optional configurations.
// Options include setting the name and interval of the Clock.
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

// ClockOption defines a function type for configuring a Clock.
type ClockOption func(*Clock)

// WithName sets the name of the Clock.
func WithName(name string) ClockOption {
	return func(c *Clock) {
		c.name = name
	}
}

// WithInterval sets the interval for the Clock ticks.
func WithInterval(interval time.Duration) ClockOption {
	return func(c *Clock) {
		c.interval = interval
	}
}

// Add registers a Ticker to the Clock with a specific ExecutionMode.
// It stores the TickerSubscriber in a sync.Map for thread-safe access.
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

// Start begins the dispatching of ticks to subscribed tickers.
func (c *Clock) Start() {
	if !c.started.CompareAndSwap(false, true) {
		return
	}
	c.ticking.Store(true)
	go c.dispatchTicks()
}

// Stop halts the dispatching of ticks and stops the Clock.
func (c *Clock) Stop() {
	if !c.ticking.CompareAndSwap(true, false) {
		return
	}
	close(c.stopCh)
	c.started.Store(false)
}

// dispatchTicks handles the periodic dispatching of ticks to subscribers.
func (c *Clock) dispatchTicks() {
	for {
		select {
		case now := <-c.ticker.C:
			c.subs.Range(func(key, value interface{}) bool {
				sub := value.(*TickerSubscriber)
				lastExecTime := sub.lastExecTime.Load().(time.Time)
				if now.Sub(lastExecTime) >= sub.DynamicInterval(now.Sub(lastExecTime)) {
					switch sub.Mode {
					case NonBlocking, BestEffort:
						go c.executeTick(key, sub, now)
					case ManagedTimeline:
						c.executeTick(key, sub, now)
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

// executeTick triggers the Tick method on a subscriber.
// It updates the last execution time and stores the subscriber back in the sync.Map.
func (c *Clock) executeTick(key interface{}, sub *TickerSubscriber, now time.Time) {
	sub.Ticker.Tick()
	sub.lastExecTime.Store(now)
	c.subs.Store(key, sub)
}

type AppManagerMsg[Key RuntimeKey] struct {
	name Key
	app  RuntimeApplication
}

// AppManager manages the lifecycle of runtime applications.
// It uses a sync.Map to store applications and their associated cancellation functions and channels.
// The generic type Key is used to uniquely identify applications.
type AppManager[Key RuntimeKey] struct {
	apps          sync.Map
	ctx           context.Context
	cancel        context.CancelFunc
	errChan       chan error
	shutdownTimer time.Duration
	once          sync.Once
	mu            sync.Mutex
	closed        bool
	useAddChan    chan AppManagerMsg[Key]
	useWaitChan   chan struct{}
}

// NewAppManager creates a new AppManager with the specified shutdown timeout.
// The shutdown timer specifies the duration to wait for applications to shut down gracefully.
func NewAppManager[Key RuntimeKey](shutdownTimer time.Duration) *AppManager[Key] {
	ctx, cancel := context.WithCancel(context.Background())
	am := &AppManager[Key]{
		ctx:           ctx,
		cancel:        cancel,
		errChan:       make(chan error, 1),
		shutdownTimer: shutdownTimer,
		closed:        false,
		useAddChan:    make(chan AppManagerMsg[Key]),
		useWaitChan:   make(chan struct{}),
	}

	go am.listenForAdd()
	return am
}

func (am *AppManager[Key]) listenForAdd() {
	for {
		select {
		case msg := <-am.useAddChan:
			_ = am.AddApplication(msg.name, msg.app)
		case <-am.ctx.Done():
			return
		}
	}
}

// AddApplication adds a new application to the AppManager.
// It initializes a context and shutdown channel for the application and starts it in a new goroutine.
// If an application with the same name already exists, it returns an error.
func (am *AppManager[Key]) AddApplication(name Key, app RuntimeApplication) error {
	appCtx := context.WithValue(am.ctx, appManagerKey, am)
	appCtx, cancelFunc := context.WithCancel(appCtx)
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
			am.mu.Lock()
			defer am.mu.Unlock()
			if !am.closed {
				select {
				case am.errChan <- fmt.Errorf("error from application %v: %w", name, err):
				default:
					slog.Error("Failed to send error to channel", "name", name, "error", err)
				}
			}
		}
	}()

	return nil
}

// ShutdownApplication shuts down an application by its name.
// It cancels the context and closes the shutdown channel.
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
	default:
		close(app.shutdownCh)
	}

	slog.Info("Application shut down", "name", name)
	return nil
}

// GracefulShutdown attempts to shut down all applications gracefully within the specified timeout.
// It waits for all applications to signal that they have shut down before returning.
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
			select {
			case <-app.shutdownCh:
				// Application already closed
			default:
				close(app.shutdownCh)
			}
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

// Run starts all provided applications and waits for a shutdown signal.
// It returns a channel to trigger shutdown and another to receive errors.
func (am *AppManager[Key]) Run(apps map[Key]RuntimeApplication) (chan struct{}, <-chan error) {
	for name, app := range apps {
		if err := am.AddApplication(name, app); err != nil {
			slog.Error("Failed to add application", "name", name, "error", err)
		}
	}

	shutdownChan := make(chan struct{})
	go func() {
		for msg := range am.useAddChan {
			if err := am.AddApplication(msg.name, msg.app); err != nil {
				slog.Error("Failed to add application via UseAdd", "name", msg.name, "error", err)
			}
		}
	}()

	go func() {
		<-shutdownChan
		if err := am.GracefulShutdown(); err != nil {
			am.mu.Lock()
			defer am.mu.Unlock()
			if !am.closed {
				select {
				case am.errChan <- fmt.Errorf("error during graceful shutdown: %w", err):
				default:
					slog.Error("Failed to send shutdown error to channel", "error", err)
				}
			}
		}
		am.cancel()
		am.once.Do(func() {
			am.mu.Lock()
			defer am.mu.Unlock()
			if !am.closed {
				close(am.errChan)
				am.closed = true
			}
		})
	}()

	return shutdownChan, am.errChan
}

type contextKey string

const appManagerKey contextKey = "appManager"

// UseAdd returns a channel to send a RuntimeApplication function and its name to the AppManager for adding.
func (am *AppManager[Key]) UseAdd() chan<- AppManagerMsg[Key] {
	return am.useAddChan
}

// UseWait returns a channel depending on the state of the specified application.
func (am *AppManager[Key]) UseWait(name Key, state string) <-chan struct{} {
	waitChan := make(chan struct{})
	go func() {
		defer func() {
			if r := recover(); r != nil {
				// handle panic if channel is already closed
			}
		}()
		defer close(waitChan)
		for {
			app, loaded := am.apps.Load(name)
			if !loaded {
				return
			}
			appData := app.(struct {
				app        RuntimeApplication
				cancelFunc context.CancelFunc
				shutdownCh chan struct{}
			})

			switch state {
			case "started":
				if am.ctx.Err() == nil {
					return
				}
			case "running":
				if am.ctx.Err() == nil && appData.shutdownCh != nil {
					return
				}
			case "completed":
				select {
				case <-appData.shutdownCh:
					return
				default:
				}
			case "error":
				select {
				case err := <-am.errChan:
					if err != nil {
						return
					}
				default:
				}
			}

			time.Sleep(50 * time.Millisecond)
		}
	}()
	return waitChan
}

// NonBlockingMiddleware creates a middleware that runs the application in NonBlocking mode.
func NonBlockingMiddleware(interval time.Duration, app RuntimeApplication) RuntimeApplication {
	return createMiddleware(interval, NonBlocking, app)
}

// ManagedTimelineMiddleware creates a middleware that runs the application in ManagedTimeline mode.
func ManagedTimelineMiddleware(interval time.Duration, app RuntimeApplication) RuntimeApplication {
	return createMiddleware(interval, ManagedTimeline, app)
}

// BestEffortMiddleware creates a middleware that runs the application in BestEffort mode.
func BestEffortMiddleware(interval time.Duration, app RuntimeApplication) RuntimeApplication {
	return createMiddleware(interval, BestEffort, app)
}

// createMiddleware wraps a RuntimeApplication with a Clock to periodically trigger its execution.
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

// tickerWrapper wraps a RuntimeApplication for periodic execution by a Clock.
type tickerWrapper struct {
	app      RuntimeApplication
	ctx      context.Context
	shutdown chan struct{}
	errCh    chan error
	cancel   context.CancelFunc
}

// Tick executes the wrapped application and handles any errors.
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
