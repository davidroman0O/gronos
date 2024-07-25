// Package gronos provides a concurrent application management system.
package gronos

import (
	"context"
	"fmt"
	"runtime"
	"sync"
	"time"

	"github.com/avast/retry-go/v3"
)

// RuntimeApplication is a function type representing an application that can be run concurrently.
// It takes a context and a shutdown channel as parameters and returns an error.
type RuntimeApplication func(ctx context.Context, shutdown <-chan struct{}) error

// message is an interface type for internal communication within gronos.
type message interface{}

// gronos is the main struct that manages concurrent applications.
// It is parameterized by a comparable key type K.
type gronos[K comparable] struct {
	applications sync.Map
	keys         ConcurrentArray[K]
	com          chan message
	wait         sync.WaitGroup
	ctx          context.Context
	cancel       context.CancelFunc
	done         chan struct{}
}

// DeadLetter represents a message indicating that an application has terminated with an error.
type DeadLetter[K comparable] struct {
	Key    K
	Reason error
}

// Terminated represents a message indicating that an application has terminated normally.
type Terminated[K comparable] struct {
	Key K
}

// ContextTerminated represents a message indicating that an application's context has been terminated.
type ContextTerminated[K comparable] struct {
	Key K
	Err error
}

// Add represents a message to add a new application to the gronos system.
type Add[K comparable] struct {
	Key K
	App RuntimeApplication
}

// applicationContext holds the context and metadata for a running application.
type applicationContext[K comparable] struct {
	k        K
	app      RuntimeApplication
	ctx      context.Context
	com      chan message
	retries  uint
	alive    bool
	reason   error
	shutdown chan struct{}
	closer   func() // proxy function to close the channel shutdown
	cancel   func() // proxy function to cancel the context
}

// New creates a new gronos instance with the given context and initial applications.
// It returns a pointer to the created gronos instance.
//
// Example usage:
//
//	ctx := context.Background()
//	apps := map[string]RuntimeApplication{
//		"app1": func(ctx context.Context, shutdown <-chan struct{}) error {
//			// Application logic here
//			return nil
//		},
//	}
//	g := gronos.New(ctx, apps)
func New[K comparable](ctx context.Context, init map[K]RuntimeApplication) *gronos[K] {
	ctx, cancel := context.WithCancel(ctx)
	g := &gronos[K]{
		com:    make(chan message, 200),
		keys:   ConcurrentArray[K]{},
		ctx:    ctx,
		cancel: cancel,
		done:   make(chan struct{}),
	}
	for k, v := range init {
		g.Add(k, v)
	}
	return g
}

// Start begins running the gronos instance and returns a channel for receiving errors.
//
// Example usage:
//
//	errChan := g.Start()
//	for err := range errChan {
//		log.Printf("Error: %v", err)
//	}
func (g *gronos[K]) Start() chan error {
	errChan := make(chan error, 1)
	go g.run(errChan)
	return errChan
}

// Shutdown initiates the shutdown process for all applications managed by the gronos instance.
//
// Example usage:
//
//	g.Shutdown()
func (g *gronos[K]) Shutdown() {
	g.cancel()
	select {
	case <-g.done:
		// Shutdown completed successfully
	case <-time.After(5 * time.Second):
		// Timeout occurred, log a warning
		fmt.Println("Warning: Shutdown timed out after 5 seconds")
	}
}

// Wait blocks until all applications managed by the gronos instance have terminated.
//
// Example usage:
//
//	g.Wait()
func (g *gronos[K]) Wait() {
	<-g.done
}

// run is the main loop of the gronos instance, handling messages and managing applications.
func (g *gronos[K]) run(errChan chan<- error) {
	defer close(g.done)
	defer close(errChan)

	for {
		select {
		case <-g.ctx.Done():
			g.shutdownApps()
			errChan <- g.ctx.Err() // Propagate the context error
			return
		case m, ok := <-g.com:
			if !ok {
				return
			}
			if err := g.handleMessage(m); err != nil {
				errChan <- err
			}
		}
		runtime.Gosched()
	}
}

// shutdownApps initiates the shutdown process for all running applications.
func (g *gronos[K]) shutdownApps() {
	var wg sync.WaitGroup
	g.applications.Range(func(key, value interface{}) bool {
		app := value.(applicationContext[K])
		if app.alive {
			wg.Add(1)
			go func() {
				defer wg.Done()
				app.cancel()
				<-app.shutdown
			}()
		}
		return true
	})
	wg.Wait()
}

// handleMessage processes incoming messages and updates the gronos state accordingly.
func (g *gronos[K]) handleMessage(m message) error {
	switch msg := m.(type) {
	case DeadLetter[K]:
		var value any
		var app applicationContext[K]
		var ok bool
		if value, ok = g.applications.Load(msg.Key); !ok {
			return nil
		}
		if app, ok = value.(applicationContext[K]); !ok {
			return nil
		}
		app.alive = false
		app.reason = msg.Reason
		app.closer()
		g.applications.Store(app.k, app)
		if msg.Reason != nil && msg.Reason.Error() != "shutdown" {
			return fmt.Errorf("app error %v %v", msg.Key, msg.Reason)
		}
	case Terminated[K]:
		var value any
		var app applicationContext[K]
		var ok bool
		if value, ok = g.applications.Load(msg.Key); !ok {
			return nil
		}
		if app, ok = value.(applicationContext[K]); !ok {
			return nil
		}
		app.alive = false
		app.closer()
		g.applications.Store(app.k, app)
	case ContextTerminated[K]:
		var value any
		var app applicationContext[K]
		var ok bool
		if value, ok = g.applications.Load(msg.Key); !ok {
			return nil
		}
		if app, ok = value.(applicationContext[K]); !ok {
			return nil
		}
		app.alive = false
		app.cancel()
		g.applications.Store(app.k, app)
	case Add[K]:
		return g.Add(msg.Key, msg.App)
	}
	return nil
}

// Add adds a new application to the gronos instance with the given key and RuntimeApplication.
// It returns an error if an application with the same key already exists or if the context is already cancelled.
//
// Example usage:
//
//	err := g.Add("newApp", func(ctx context.Context, shutdown <-chan struct{}) error {
//		// New application logic here
//		return nil
//	})
//	if err != nil {
//		log.Printf("Failed to add new application: %v", err)
//	}
func (g *gronos[K]) Add(k K, v RuntimeApplication) error {
	if _, ok := g.applications.Load(k); ok {
		return fmt.Errorf("application with key %v already exists", k)
	}
	if g.ctx.Err() != nil {
		return fmt.Errorf("context is already cancelled")
	}

	ctx, cancel := g.createContext()
	shutdown := make(chan struct{})

	appCtx := applicationContext[K]{
		k:        k,
		app:      v,
		ctx:      ctx,
		com:      g.com,
		retries:  0,
		shutdown: shutdown,
		reason:   nil,
		alive:    true,
	}

	appCtx.closer = sync.OnceFunc(func() {
		close(shutdown)
		g.wait.Done()
	})

	appCtx.cancel = sync.OnceFunc(func() {
		cancel()
		appCtx.closer()
	})

	g.applications.Store(k, appCtx)
	g.keys.Append(k)
	g.wait.Add(1)

	go func(key K, ctx applicationContext[K]) {
		var err error
		if ctx.retries == 0 {
			err = ctx.app(ctx.ctx, ctx.shutdown)
		} else {
			err = retry.Do(func() error {
				return ctx.app(ctx.ctx, ctx.shutdown)
			}, retry.Attempts(ctx.retries))
		}
		if err != nil && err != context.Canceled {
			select {
			case g.com <- DeadLetter[K]{Key: key, Reason: err}:
			case <-g.ctx.Done():
			}
		} else if err == context.Canceled {
			select {
			case g.com <- ContextTerminated[K]{Key: key, Err: ctx.ctx.Err()}:
			case <-g.ctx.Done():
			}
		} else {
			select {
			case g.com <- Terminated[K]{Key: key}:
			case <-g.ctx.Done():
			}
		}
	}(k, appCtx)

	return nil
}

type ctxKey string

var comKey ctxKey = "com"

// createContext creates a new context with the gronos communication channel embedded.
func (g *gronos[K]) createContext() (context.Context, context.CancelFunc) {
	ctx := context.WithValue(context.Background(), comKey, g.com)
	ctx, cancel := context.WithCancel(ctx)
	return ctx, cancel
}

// UseBus retrieves the communication channel from a context created by gronos.
// It returns an error if the communication channel is not found in the context.
//
// Example usage:
//
//	bus, err := gronos.UseBus(ctx)
//	if err != nil {
//		log.Printf("Failed to get communication bus: %v", err)
//		return
//	}
//	bus <- someMessage
func UseBus(ctx context.Context) (chan<- message, error) {
	value := ctx.Value(comKey)
	if value == nil {
		return nil, fmt.Errorf("com not found in context")
	}
	return value.(chan message), nil
}

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
