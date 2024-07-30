// Package gronos provides a concurrent application management system.
package gronos

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/avast/retry-go/v3"
)

type ShutdownBehavior int

const (
	ShutdownAutomatic ShutdownBehavior = iota
	ShutdownManual
)

// StatusState represents the possible states of a component
type StatusState string

const (
	StatusAdded     StatusState = "added"
	StatusStarting  StatusState = "starting"
	StatusRunning   StatusState = "running"
	StatusCompleted StatusState = "completed"
	StatusFailed    StatusState = "failed"
)

// StatusMessage is used to update the status of a component
type StatusMessage[K comparable] struct {
	HeaderMessage[K]
	State StatusState
}

// StatusResponseMessage is the response to a status request
type StatusResponseMessage[K comparable] struct {
	HeaderMessage[K]
	State StatusState
}

// RuntimeApplication is a function type representing an application that can be run concurrently.
// It takes a context and a shutdown channel as parameters and returns an error.
type RuntimeApplication func(ctx context.Context, shutdown <-chan struct{}) error

// Message is an interface type for internal communication within gronos.
type Message interface{}

type gronosConfig struct {
	shutdownBehavior ShutdownBehavior
	gracePeriod      time.Duration
	minRuntime       time.Duration
}

// gronos is the main struct that manages concurrent applications.
// It is parameterized by a comparable key type K.
type gronos[K comparable] struct {
	applications    sync.Map
	keys            ConcurrentArray[K]
	com             chan Message
	wait            sync.WaitGroup
	ctx             context.Context
	cancel          context.CancelFunc
	done            chan struct{}
	closer          func()
	cancelled       atomic.Bool
	extensions      []ExtensionHooks[K]
	statuses        sync.Map
	config          gronosConfig
	startTime       time.Time
	requestShutdown chan struct{}
}

// applicationContext holds the context and metadata for a running application.
type applicationContext[K comparable] struct {
	k        K
	app      RuntimeApplication
	ctx      context.Context
	com      chan Message
	retries  uint
	alive    bool
	reason   error
	shutdown chan struct{}
	closer   func() // proxy function to close the channel shutdown
	cancel   func() // proxy function to cancel the context
}

type Option[K comparable] func(*gronos[K])

func WithExtension[K comparable](ext ExtensionHooks[K]) Option[K] {
	return func(ctx *gronos[K]) {
		ctx.extensions = append(ctx.extensions, ext)
	}
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
func New[K comparable](ctx context.Context, init map[K]RuntimeApplication, opts ...Option[K]) *gronos[K] {
	ctx, cancel := context.WithCancel(ctx)
	g := &gronos[K]{
		com:        make(chan Message, 200),
		keys:       ConcurrentArray[K]{},
		ctx:        ctx,
		cancel:     cancel,
		done:       make(chan struct{}),
		extensions: []ExtensionHooks[K]{},
		statuses:   sync.Map{},
		config: gronosConfig{
			shutdownBehavior: ShutdownAutomatic,
			gracePeriod:      2 * time.Second,
			minRuntime:       2 * time.Second,
		},
		requestShutdown: make(chan struct{}),
	}
	for _, opt := range opts {
		opt(g)
	}
	for k, v := range init {
		g.Add(k, v)
	}
	g.closer = sync.OnceFunc(func() {
		close(g.com)
		close(g.done)
		close(g.requestShutdown)
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
	errChan := make(chan error, 100)
	g.cancelled.Store(false)
	g.startTime = time.Now()

	// Apply extensions' OnStart hooks
	for _, ext := range g.extensions {
		if err := ext.OnStart(g.ctx, errChan); err != nil {
			errChan <- fmt.Errorf("extension error on start: %w", err)
			close(errChan)
			return errChan
		}
	}

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
	fmt.Println("gronos shutdown function called")
	g.shutdownApps(false)
	if c.timeout > 0 {
		<-time.After(c.timeout)
	}
	g.com <- ShutdownMessage{}
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
	defer func() {
		if g.ctx.Err() == context.Canceled {
			errChan <- g.ctx.Err()
		}
		fmt.Println("gronos run done")
		// Apply extensions' OnStop hooks
		for _, ext := range g.extensions {
			fmt.Println("gronos extension stop")
			if err := ext.OnStop(g.ctx, errChan); err != nil {
				errChan <- fmt.Errorf("extension error on stop: %w", err)
			}
		}
		close(errChan)
		g.closer()
	}()

	ticker := time.NewTicker(1 * time.Second)

	for {
		select {
		case <-g.ctx.Done():
			if g.cancelled.Load() {
				continue
			}
			g.cancelled.Store(true)
			g.shutdownApps(true)
			g.cancelled.Store(true)
			g.com <- msgContextCancelled[K]()
			continue

		case <-g.requestShutdown:
			fmt.Println("gronos request shutdown message")
			if g.config.shutdownBehavior == ShutdownManual {
				fmt.Println("gronos request shutdown message returned")
				ticker.Stop() // just in case it wasn't clear before
				return
			}
			// sorry but you're in automatic mode
			continue

		case <-ticker.C:
			// no ticker for manual
			if g.config.shutdownBehavior == ShutdownManual {
				ticker.Stop()
				continue
			}

			fmt.Println("gronos ticker should consider")
			if g.shouldConsiderShutdown() {
				if g.considerShutdown() {
					ticker.Stop()
					return
				}
			}

		case m, ok := <-g.com:
			fmt.Println("gronos message", m)
			if !ok {
				return
			}

			if err := g.handleMessage(m); err != nil {
				errChan <- err
			}
		}
	}
}

func (g *gronos[K]) shouldConsiderShutdown() bool {
	if g.config.shutdownBehavior == ShutdownManual {
		fmt.Println("gronos manual shutdown")
		return false
	}

	fmt.Println("time since", time.Since(g.startTime), g.config.minRuntime)
	if time.Since(g.startTime) < g.config.minRuntime {
		return false
	}

	howMuchAlive := 0
	g.applications.Range(func(key, value interface{}) bool {
		app := value.(applicationContext[K])
		if app.alive {
			howMuchAlive++
		}
		return true
	})

	fmt.Println("how much alive", howMuchAlive)

	return howMuchAlive == 0
}

func (g *gronos[K]) considerShutdown() bool {
	fmt.Println("gronos consider shutdown")
	select {
	case <-time.After(g.config.gracePeriod):
		fmt.Println("gronos grace period")
		g.com <- ShutdownMessage{}
		return true
	case <-g.ctx.Done():
		fmt.Println("gronos context cancelled no shutdown")
		// Context cancelled, no need to send shutdown signal
		return false
	}
}

// shutdownApps initiates the shutdown process for all running applications.
func (g *gronos[K]) shutdownApps(cancelled bool) {
	var wg sync.WaitGroup
	g.applications.Range(func(key, value interface{}) bool {
		app := value.(applicationContext[K])
		if app.alive {
			wg.Add(1)
			fmt.Println("gronos shutdown", app.k)
			if !cancelled {
				g.com <- MsgDeadLetter(app.k, errors.New("shutdown"))
			} else {
				g.com <- MsgContextTerminated(app.k, nil)
			}
			wg.Done()
		}
		return true
	})
	fmt.Println("gronos shutdown wait")
	wg.Wait()
	fmt.Println("gronos shutdown done")
}

// handleMessage processes incoming messages and updates the gronos state accordingly.
func (g *gronos[K]) handleMessage(m Message) error {
	// First, try to handle the message with the gronos core
	err := g.handleGronosMessage(m)
	if err == nil {
		return nil
	}

	// If the gronos core couldn't handle it, pass it to extensions
	for _, ext := range g.extensions {
		err := ext.OnMsg(g.ctx, m)
		if err == nil {
			return nil
		}
	}

	return fmt.Errorf("unhandled message type: %T", m)
}

// IsStarted checks if a component has started
func (g *gronos[K]) IsStarted(k K) bool {
	state, ok := g.statuses.Load(k)
	return ok && (state == StatusStarting || state == StatusRunning)
}

// IsComplete checks if a component has completed
func (g *gronos[K]) IsComplete(k K) bool {
	state, ok := g.statuses.Load(k)
	return ok && state == StatusCompleted
}

// GetStatus retrieves the current status of a component
func (g *gronos[K]) GetStatus(k K) (StatusState, bool) {
	state, ok := g.statuses.Load(k)
	if !ok {
		return "", false
	}
	return state.(StatusState), true
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
	if g.cancelled.Load() {
		return fmt.Errorf("context is already cancelled")
	}

	ctx, cancel := g.createContext()

	// runtime application shutdown channel
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
		cancel() // cancel first
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
		fmt.Println("gronos application done", key, err)
		var value any
		var ok bool
		if value, ok = g.applications.Load(key); !ok {
			// quite critical
			g.com <- MsgError(key, fmt.Errorf("unable to find application %v", key))
			return
		}
		future := value.(applicationContext[K])
		// that mean the application was requested from outside to be terminated as a shutdown
		if !future.alive {
			return
		}
		// async termination
		if err != nil && err != context.Canceled {
			g.com <- MsgDeadLetter(key, err)
		} else if err == context.Canceled {
			g.com <- MsgContextTerminated(key, ctx.ctx.Err())
		} else {
			g.com <- MsgTerminated(key)
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
func UseBus(ctx context.Context) (chan<- Message, error) {
	value := ctx.Value(comKey)
	if value == nil {
		return nil, fmt.Errorf("com not found in context")
	}
	return value.(chan Message), nil
}

func WithShutdownBehavior[K comparable](behavior ShutdownBehavior) Option[K] {
	return func(g *gronos[K]) {
		g.config.shutdownBehavior = behavior
	}
}

func WithGracePeriod[K comparable](period time.Duration) Option[K] {
	return func(g *gronos[K]) {
		g.config.gracePeriod = period
	}
}

func WithMinRuntime[K comparable](duration time.Duration) Option[K] {
	return func(g *gronos[K]) {
		g.config.minRuntime = duration
	}
}

// Manual shutdown function
func (g *gronos[K]) RequestShutdown() {
	g.com <- ShutdownMessage{}
}
