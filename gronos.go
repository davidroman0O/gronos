// Package gronos provides a concurrent application management system.
package gronos

import (
	"context"
	"errors"
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
	closer       func()
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

type Error[K comparable] struct {
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
	g.closer = sync.OnceFunc(func() {
		close(g.com)
		close(g.done)
	})
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

type shutdownOptions struct {
	timeout time.Duration
}

type ShutdownOption func(*shutdownOptions)

func WithTimeout(timeout time.Duration) func(*shutdownOptions) {
	return func(c *shutdownOptions) {
		c.timeout = timeout
	}
}

// Shutdown initiates the shutdown process for all applications managed by the gronos instance.
//
// Example usage:
//
//	g.Shutdown()
func (g *gronos[K]) Shutdown(opts ...ShutdownOption) {
	c := &shutdownOptions{}
	for _, opt := range opts {
		opt(c)
	}
	g.shutdownApps(false)
	if c.timeout > 0 {
		<-time.After(c.timeout)
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

// Return the done channel, will be closed when all runtimes have terminated.
func (g *gronos[K]) OnDone() <-chan struct{} {
	return g.done
}

// run is the main loop of the gronos instance, handling messages and managing applications.
func (g *gronos[K]) run(errChan chan<- error) {
	defer g.closer()

	for {
		select {
		case <-g.ctx.Done():
			g.shutdownApps(true)
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
func (g *gronos[K]) shutdownApps(cancelled bool) {
	var wg sync.WaitGroup
	g.applications.Range(func(key, value interface{}) bool {
		app := value.(applicationContext[K])
		if app.alive {
			wg.Add(1)
			if !cancelled {
				g.com <- DeadLetter[K]{Key: app.k, Reason: errors.New("shutdown")}
			} else {
				g.com <- ContextTerminated[K]{Key: app.k}
			}
			wg.Done()
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
		if !app.alive {
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
		if !app.alive {
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
		if !app.alive {
			return nil
		}
		app.alive = false
		app.cancel()
		g.applications.Store(app.k, app)
	case Add[K]:
		return g.Add(msg.Key, msg.App)
	case Error[K]:
		return fmt.Errorf("app error %v %v", msg.Key, msg.Err)
	}

	howMuchAlive := 0
	g.applications.Range(func(key, value interface{}) bool {
		app := value.(applicationContext[K])
		if app.alive {
			howMuchAlive++
			return true
		}
		return true
	})

	// we somehow closed all apps, we shouldn't wait anymore
	if howMuchAlive == 0 {
		g.closer() // it's the end
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

	// despite being triggered for termination, we need to wait for the application to REALLY finish
	realDone := make(chan struct{})

	appCtx.closer = sync.OnceFunc(func() {
		close(shutdown)
		<-realDone
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
		var value any
		var ok bool
		if value, ok = g.applications.Load(key); !ok {
			// quite critical
			g.com <- Error[K]{Key: key, Err: fmt.Errorf("unable to find application %v", key)}
			return
		}
		future := value.(applicationContext[K])
		// that mean the application was requested from outside to be terminated as a shutdown
		if !future.alive {
			return
		}
		// async termination
		if err != nil && err != context.Canceled {
			g.com <- DeadLetter[K]{Key: key, Reason: err}
		} else if err == context.Canceled {
			g.com <- ContextTerminated[K]{Key: key, Err: ctx.ctx.Err()}
		} else {
			g.com <- Terminated[K]{Key: key}
		}

		close(realDone)
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
