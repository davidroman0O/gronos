package gronos

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"
)

type AppState int

const (
	StateStarted AppState = iota
	StateRunning
	StateCompleted
	StateError
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

func (c *Clock) executeTick(key interface{}, sub *TickerSubscriber, now time.Time) {
	sub.Ticker.Tick()
	sub.lastExecTime.Store(now)
	c.subs.Store(key, sub)
}

type AppManagerMsg[Key RuntimeKey] struct {
	name   Key
	app    RuntimeApplication
	parent Key
}

type AppManager[Key RuntimeKey] struct {
	apps            sync.Map
	errChan         chan error
	ctx             context.Context
	cancel          context.CancelFunc
	useAddChan      chan AppManagerMsg[Key]
	useShutdownChan chan Key
	shutdownTimer   time.Duration
	mu              sync.Mutex
	closed          bool
	once            sync.Once
	childApps       map[Key][]Key
	childAppsMutex  sync.RWMutex
}

func NewAppManager[Key RuntimeKey](timeout time.Duration) *AppManager[Key] {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	return &AppManager[Key]{
		errChan:         make(chan error),
		ctx:             ctx,
		cancel:          cancel,
		useAddChan:      make(chan AppManagerMsg[Key]),
		useShutdownChan: make(chan Key),
		shutdownTimer:   timeout,
		childApps:       make(map[Key][]Key),
	}
}

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

	am.childAppsMutex.Lock()
	am.childApps[name] = []Key{}
	am.childAppsMutex.Unlock()

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

func (am *AppManager[Key]) ShutdownApplication(name Key) error {
	am.childAppsMutex.RLock()
	childApps := am.childApps[name]
	am.childAppsMutex.RUnlock()

	for _, childName := range childApps {
		if err := am.ShutdownApplication(childName); err != nil {
			if !errors.Is(err, ErrApplicationNotFound) {
				slog.Error("Failed to shutdown child application", "parent", name, "child", childName, "error", err)
			}
		}
	}

	value, loaded := am.apps.LoadAndDelete(name)
	if !loaded {
		return ErrApplicationNotFound
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

	am.childAppsMutex.Lock()
	delete(am.childApps, name)
	am.childAppsMutex.Unlock()

	slog.Info("Application shut down", "name", name)
	return nil
}

// Add this error to gronos.go:
var ErrApplicationNotFound = errors.New("application not found")

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

func (am *AppManager[Key]) Run(apps map[Key]RuntimeApplication) (chan struct{}, <-chan error) {
	for name, app := range apps {
		if err := am.AddApplication(name, app); err != nil {
			fmt.Printf("Failed to add application %v: %v\n", name, err)
		}
	}

	shutdownChan := make(chan struct{})

	go func() {
		for msg := range am.useAddChan {
			if err := am.AddApplication(msg.name, msg.app); err != nil {
				fmt.Printf("Failed to add application via UseAdd %v: %v\n", msg.name, err)
			} else {
				am.childAppsMutex.Lock()
				am.childApps[msg.parent] = append(am.childApps[msg.parent], msg.name)
				am.childAppsMutex.Unlock()
			}
		}
	}()

	go func() {
		for name := range am.useShutdownChan {
			if err := am.ShutdownApplication(name); err != nil {
				fmt.Printf("Failed to shut down application via UseShutdown %v: %v\n", name, err)
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
					fmt.Printf("Failed to send shutdown error to channel: %v\n", err)
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
const currentAppKey contextKey = "currentApp"

func UseAdd[Key RuntimeKey](ctx context.Context, addChan chan<- AppManagerMsg[Key], name Key, app RuntimeApplication) {
	var parent Key
	if parentValue := ctx.Value(currentAppKey); parentValue != nil {
		if p, ok := parentValue.(Key); ok {
			parent = p
		}
	}

	select {
	case addChan <- AppManagerMsg[Key]{name: name, app: app, parent: parent}:
	case <-ctx.Done():
	}
}

func UseWait[Key RuntimeKey](ctx context.Context, am *AppManager[Key], name Key, state AppState) <-chan struct{} {
	waitChan := make(chan struct{})
	go func() {
		defer close(waitChan)
		for {
			select {
			case <-ctx.Done():
				return
			default:
			}

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
			case StateStarted:
				if appData.shutdownCh != nil {
					return
				}
			case StateRunning:
				if appData.shutdownCh != nil && am.ctx.Err() == nil {
					return
				}
			case StateCompleted:
				select {
				case <-appData.shutdownCh:
					return
				default:
				}
			case StateError:
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

func UseShutdown[Key RuntimeKey](ctx context.Context, shutdownChan chan<- Key, name Key) {
	select {
	case shutdownChan <- name:
	case <-ctx.Done():
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
