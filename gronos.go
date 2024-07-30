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
	applications sync.Map
	keys         ConcurrentArray[K]
	com          chan Message
	wait         sync.WaitGroup
	ctx          context.Context
	cancel       context.CancelFunc
	done         chan struct{}
	closer       func()
	isShutting   atomic.Bool
	extensions   []ExtensionHooks[K]
	statuses     sync.Map
	config       gronosConfig
	startTime    time.Time
}

// applicationContext holds the context and metadata for a running application.
type applicationContext[K comparable] struct {
	k        K
	app      RuntimeApplication
	ctx      context.Context
	com      chan Message
	retries  uint
	alive    atomic.Bool
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
	})
	return g
}

// Start begins running the gronos instance and returns a channel for receiving errors.
func (g *gronos[K]) Start() chan error {
	errChan := make(chan error, 100)
	g.isShutting.Store(false)
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

func WithTimeout(timeout time.Duration) ShutdownOption {
	return func(c *shutdownOptions) {
		c.timeout = timeout
	}
}

// Shutdown initiates the shutdown process for all applications managed by the gronos instance.
func (g *gronos[K]) Shutdown(opts ...ShutdownOption) {
	c := &shutdownOptions{}
	for _, opt := range opts {
		opt(c)
	}
	g.com <- ShutdownMessage{}
	if c.timeout > 0 {
		<-time.After(c.timeout)
	}
}

// Wait blocks until all applications managed by the gronos instance have terminated.
func (g *gronos[K]) Wait() {
	<-g.done
}

// OnDone returns the done channel, which will be closed when all runtimes have terminated.
func (g *gronos[K]) OnDone() <-chan struct{} {
	return g.done
}

// run is the main loop of the gronos instance, handling messages and managing applications.
func (g *gronos[K]) run(errChan chan<- error) {
	defer func() {
		// Apply extensions' OnStop hooks
		for _, ext := range g.extensions {
			if err := ext.OnStop(g.ctx, errChan); err != nil {
				errChan <- fmt.Errorf("extension error on stop: %w", err)
			}
		}
		close(errChan)
		g.closer()
	}()

	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-g.ctx.Done():
			g.initiateShutdown(true)
			return
		case <-ticker.C:
			if g.config.shutdownBehavior == ShutdownAutomatic {
				g.checkAutomaticShutdown()
			}
		case m, ok := <-g.com:
			if !ok {
				return
			}
			if err := g.handleMessage(m); err != nil {
				errChan <- err
			}
		}
	}
}

func (g *gronos[K]) checkAutomaticShutdown() {
	if time.Since(g.startTime) < g.config.minRuntime {
		return
	}

	allStopped := true
	g.applications.Range(func(_, value interface{}) bool {
		app := value.(applicationContext[K])
		if app.alive.Load() {
			allStopped = false
			return false
		}
		return true
	})

	if allStopped {
		g.initiateShutdown(false)
	}
}

func (g *gronos[K]) initiateShutdown(cancelled bool) {
	if !g.isShutting.CompareAndSwap(false, true) {
		return // Shutdown already in progress
	}

	if g.config.shutdownBehavior == ShutdownManual && !cancelled {
		g.shutdownApps(false)
	} else {
		timer := time.NewTimer(g.config.gracePeriod)
		defer timer.Stop()

		shutdownChan := make(chan struct{})
		go g.gracefulShutdown(shutdownChan)

		select {
		case <-timer.C:
			g.forceShutdown()
		case <-shutdownChan:
			// Graceful shutdown completed
		}
	}
	g.cancel() // Cancel the context after shutdown is complete
}

func (g *gronos[K]) gracefulShutdown(done chan<- struct{}) {
	g.shutdownApps(false)
	close(done)
}

func (g *gronos[K]) forceShutdown() {
	g.applications.Range(func(_, value interface{}) bool {
		app := value.(applicationContext[K])
		if app.alive.Load() {
			app.cancel()
		}
		return true
	})
}

// shutdownApps initiates the shutdown process for all running applications.
func (g *gronos[K]) shutdownApps(cancelled bool) {
	var wg sync.WaitGroup
	g.applications.Range(func(key, value interface{}) bool {
		app := value.(applicationContext[K])
		if app.alive.Load() {
			wg.Add(1)
			go func(k K) {
				defer wg.Done()
				if cancelled {
					g.com <- MsgContextTerminated(k, g.ctx.Err())
				} else {
					g.com <- MsgDeadLetter(k, errors.New("shutdown"))
				}
			}(app.k)
		}
		return true
	})
	wg.Wait()
}

// handleMessage processes incoming messages and updates the gronos state accordingly.
func (g *gronos[K]) handleMessage(m Message) error {
	// Try to handle the message with the gronos core
	coreErr := g.handleGronosMessage(m)

	// If the gronos core couldn't handle it or returned an error, pass it to extensions
	if coreErr != nil {
		for _, ext := range g.extensions {
			extErr := ext.OnMsg(g.ctx, m)
			if extErr == nil {
				// Message was handled by an extension
				return nil
			}
			// Collect extension errors, but continue trying other extensions
			coreErr = errors.Join(coreErr, extErr)
		}
	}

	// If the message wasn't handled by core or any extension, return an error
	if errors.Is(coreErr, ErrUnhandledMessage) {
		return fmt.Errorf("unhandled message type: %T", m)
	}

	// Return any errors encountered during message handling
	return coreErr
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
func (g *gronos[K]) Add(k K, v RuntimeApplication) error {
	if _, ok := g.applications.Load(k); ok {
		return fmt.Errorf("application with key %v already exists", k)
	}
	if g.ctx.Err() != nil {
		return fmt.Errorf("context is already cancelled")
	}
	if g.isShutting.Load() {
		return fmt.Errorf("gronos is shutting down")
	}

	ctx, cancel := g.createContext()
	shutdown := make(chan struct{})

	// extend context with new eventual context
	for _, ext := range g.extensions {
		ctx = ext.OnNewRuntime(ctx)
	}

	appCtx := applicationContext[K]{
		k:        k,
		app:      v,
		ctx:      ctx,
		com:      g.com,
		retries:  0,
		shutdown: shutdown,
		reason:   nil,
	}
	appCtx.alive.Store(true)

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

		value, ok := g.applications.Load(key)
		if !ok {
			g.com <- MsgError(key, fmt.Errorf("unable to find application %v", key))
			return
		}
		future := value.(applicationContext[K])
		if !future.alive.Load() {
			return
		}

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

// RequestShutdown initiates a manual shutdown
func (g *gronos[K]) RequestShutdown() {
	g.com <- ShutdownMessage{}
}
