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
	_, loaded := am.apps.LoadOrStore(name, struct {
		app        RuntimeApplication
		cancelFunc context.CancelFunc
		shutdownCh chan struct{}
	}{app: app, cancelFunc: nil, shutdownCh: make(chan struct{})})

	if loaded {
		return fmt.Errorf("application with name %v already exists", name)
	}

	appCtx := context.WithValue(am.ctx, appManagerKey, am)
	appCtx = context.WithValue(appCtx, currentAppKey, name)
	appCtx, cancelFunc := context.WithCancel(appCtx)

	value, _ := am.apps.Load(name)
	appStruct := value.(struct {
		app        RuntimeApplication
		cancelFunc context.CancelFunc
		shutdownCh chan struct{}
	})
	appStruct.cancelFunc = cancelFunc
	am.apps.Store(name, appStruct)

	am.childAppsMutex.Lock()
	am.childApps[name] = []Key{}
	am.childAppsMutex.Unlock()

	go func() {
		defer func() {
			cancelFunc()
			am.childAppsMutex.Lock()
			delete(am.childApps, name)
			am.childAppsMutex.Unlock()
			am.apps.Delete(name)
			slog.Info("Application finished", "name", name)
		}()

		if err := app(appCtx, appStruct.shutdownCh); err != nil {
			select {
			case am.errChan <- fmt.Errorf("error from application %v: %w", name, err):
			default:
				go func() {
					am.errChan <- fmt.Errorf("error from application %v: %w", name, err)
				}()
			}
		}
	}()

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

	am.childAppsMutex.RLock()
	childApps := am.childApps[name]
	am.childAppsMutex.RUnlock()

	for _, childName := range childApps {
		if err := am.ShutdownApplication(childName); err != nil {
			slog.Error("Failed to shutdown child application", "parent", name, "child", childName, "error", err)
		}
	}

	am.childAppsMutex.Lock()
	delete(am.childApps, name)
	am.childAppsMutex.Unlock()

	slog.Info("Application shut down", "name", name)
	return nil
}

func (am *AppManager[Key]) Run(apps map[Key]RuntimeApplication) (chan struct{}, <-chan error) {
	for name, app := range apps {
		if err := am.AddApplication(name, app); err != nil {
			slog.Error("Failed to add application", "name", name, "error", err)
		}
	}

	shutdownChan := make(chan struct{})
	errChan := make(chan error, 100) // Buffered channel to avoid blocking

	var mu sync.Mutex
	go func() {
		for {
			select {
			case msg := <-am.useAddChan:
				mu.Lock()
				if err := am.AddApplication(msg.name, msg.app); err != nil {
					slog.Error("Failed to add application via UseAdd", "name", msg.name, "error", err)
				} else {
					am.childAppsMutex.Lock()
					am.childApps[msg.parent] = append(am.childApps[msg.parent], msg.name)
					am.childAppsMutex.Unlock()
				}
				mu.Unlock()
			case name := <-am.useShutdownChan:
				mu.Lock()
				if err := am.ShutdownApplication(name); err != nil {
					slog.Error("Failed to shut down application via UseShutdown", "name", name, "error", err)
				}
				mu.Unlock()
			case <-shutdownChan:
				mu.Lock()
				if err := am.GracefulShutdown(); err != nil {
					select {
					case errChan <- fmt.Errorf("error during graceful shutdown: %w", err):
					default:
						slog.Error("Failed to send shutdown error to channel", "error", err)
					}
				}
				am.cancel()
				close(errChan)
				mu.Unlock()
				return
			}
		}
	}()

	return shutdownChan, errChan
}
