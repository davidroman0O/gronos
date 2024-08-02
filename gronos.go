// Package gronos provides a concurrent application management system.
package gronos

import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"github.com/charmbracelet/log"
)

type ctxKey string

var comKey ctxKey = "com"
var keyKey ctxKey = "key"

type ShutdownBehavior int

const (
	ShutdownAutomatic ShutdownBehavior = iota
	ShutdownManual
)

var (
	ErrUnhandledMessage = errors.New("unhandled message")
	ErrPanic            = errors.New("panic")
)

// StatusState represents the possible states of a component
type StatusState string

/// States:
/// - Added: The initial state when the application is stored.
/// - Running: The state when the runtime application is triggered.
/// - ShuttingDown: The intermediate state when any shutdown mechanism is initiated.
/// - Cancelled: The state when the context is cancelled.
/// - Panicked: The state when there is panic recovery with an error.
/// - Terminated: The state when the application has finished its work.
/// - ShutdownWithError: The state when there is an error associated with the shutdown.
/// - ShutdownByRequested: The state when the framework is triggered by developer code to shut down.
/// - ShutdownByOS: The state when the operating system requests a shutdown.
/// Messages:
/// - AddRuntimeApplicationMessage: Transitions from Added to Running.
/// - CancelShutdown: Transitions from Running to Cancelled.
/// - TerminateShutdown: Transitions from Running to Terminated.
/// - PanicShutdown: Transitions from Running to Panicked.
/// - ErrorShutdown: Transitions from Running to ShutdownWithError.
/// - RequestedShutdown: Transitions from Running to ShutdownByRequested.
/// - OSShutdown: Transitions from Running to ShutdownByOS.

const (
	StatusAdded       StatusState = "added"        // first state
	StatusRunning     StatusState = "running"      // when the runtime application is triggered
	StatusShutingDown StatusState = "shuting_down" // when any of messages related to shutdown is initiated

	// final states

	StatusShutdownCancelled  StatusState = "shutdown_cancelled"
	StatusShutdownPanicked   StatusState = "shutdown_panicked"
	StatusShutdownTerminated StatusState = "shutdown_terminated"
	StatusShutdownError      StatusState = "shutdown_error"
)

func stateNumber(state StatusState) int {
	switch state {
	case StatusAdded:
		return 0
	case StatusRunning:
		return 1
	case StatusShutingDown:
		return 2
	case StatusShutdownCancelled:
		return 3
	case StatusShutdownPanicked:
		return 3
	case StatusShutdownTerminated:
		return 3
	case StatusShutdownError:
		return 3
	}
	return -1
}

// RuntimeApplication is a function type representing an application that can be run concurrently.
// It takes a context and a shutdown channel as parameters and returns an error.
type RuntimeApplication func(ctx context.Context, shutdown <-chan struct{}) error

type gronosConfig struct {
	shutdownBehavior ShutdownBehavior
	gracePeriod      time.Duration
	minRuntime       time.Duration
}

// gronos is the main struct that manages concurrent applications.
// It is parameterized by a comparable key type K.
type gronos[K comparable] struct {
	com        chan Message
	wait       sync.WaitGroup
	ctx        context.Context
	cancel     context.CancelFunc
	closer     func()
	config     gronosConfig
	startTime  time.Time
	started    atomic.Bool // define if gronos started or not
	extensions []ExtensionHooks[K]
	errChan    chan error

	// when we are shutting down to prevent triggering multiple shutdowns
	isShutting atomic.Bool

	// phase 1 - asked for shutdown
	askShutting chan struct{}
	// phase 2 - trigger on goroutines when have to shutdown
	shutting chan struct{}
	// phase 3 - when shutting is return all goroutines, closer will close the done channel
	done chan struct{}

	init map[K]RuntimeApplication

	// TODO: i think i shouldn't have `applications` but split it for each attributes and use messages to mutate it
	mkeys   sync.Map
	mapp    sync.Map // application func - key is mkey
	mctx    sync.Map // ctx - key is mkey
	mcom    sync.Map // com chan - key is mkey
	mret    sync.Map // retries - key is mkey
	mshu    sync.Map // shutdown chan - key is mkey
	mali    sync.Map // alive - key is mkey
	mrea    sync.Map // reason - key is mkey
	mcloser sync.Map // closer - key is mkey
	mcancel sync.Map // cancel - key is mkey
	mstatus sync.Map // status - key is mkey
	mdone   sync.Map // done - key is mkey
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
		com:    make(chan Message, 200),
		ctx:    ctx,
		cancel: cancel,
		// phase 1 - asked for shutdown
		askShutting: make(chan struct{}),
		// phase 2 - trigger on goroutines when have to shutdown
		shutting: make(chan struct{}),
		// phase 3 - when shutting is return all goroutines, closer will close the done channel
		done:       make(chan struct{}),
		errChan:    make(chan error, 100),
		extensions: []ExtensionHooks[K]{},
		config: gronosConfig{
			// TODO: make it configurable
			// TODO: make a sub shutdown struct
			shutdownBehavior: ShutdownAutomatic,
			gracePeriod:      2 * time.Second,
			minRuntime:       2 * time.Second,
		},
		init: init,
	}
	for _, opt := range opts {
		opt(g)
	}
	g.closer = sync.OnceFunc(func() {
		close(g.com)
		close(g.done)
		g.started.Store(false) // we stopped
	})
	return g
}

// Start begins running the gronos instance and returns a channel for receiving errors.
func (g *gronos[K]) Start() chan error {
	// can't trigger twice in a row Start until it is shutdown
	if g.started.Load() {
		g.errChan <- fmt.Errorf("gronos is already running")
		return g.errChan
	}
	// If it was shutdown, we need to re-init values
	if g.isShutting.Load() {
		// re-init values, we are re-using the same instance
		g.askShutting = make(chan struct{})
	}
	g.isShutting.Store(false)
	g.startTime = time.Now()
	g.started.Store(true)

	// Apply extensions' OnStart hooks
	for _, ext := range g.extensions {
		if err := ext.OnStart(g.ctx, g.errChan); err != nil {
			g.errChan <- fmt.Errorf("extension error on start: %w", err)
			close(g.errChan) // special case, we can't continue
			return g.errChan
		}
	}

	for k, v := range g.init {
		g.com <- MsgAddRuntimeApplication[K](k, v)
	}

	go g.readMessages(g.errChan)
	go g.run(g.errChan)

	if g.config.shutdownBehavior == ShutdownAutomatic {
		go g.automaticShutdown()
	}

	return g.errChan
}

// Shutdown initiates the shutdown process for all applications managed by the gronos instance.
func (g *gronos[K]) Shutdown() {
	log.Info("[Gronos] shutdown called")
	if g.isShutting.Load() {
		log.Error("[Gronos] shutdown already called")
		return
	}
	log.Info("[Gronos] shutdown isShutting to ", true)
	// prevent multiple shutdowns
	g.isShutting.Store(true)
	log.Info("[Gronos] shutdown trigger askShutting ", g.askShutting)
	if g.config.shutdownBehavior == ShutdownManual {
		// in that exact order
		close(g.askShutting)
		g.shutdownAllAndWait()
		close(g.shutting)
		return
	}
	// shutting down process will be kicked off by the automaticShutdown goroutine
	g.askShutting <- struct{}{}
}

// Wait blocks until all applications managed by the gronos instance have terminated.
func (g *gronos[K]) Wait() {
	log.Info("[Gronos] wait", g.done)
	if !g.started.Load() {
		// wasn't even started
		log.Info("[Gronos] wasn't started")
		g.closer()
		return
	}
	_, ok := <-g.done
	log.Info("[Gronos] wait done", ok)
}

// OnDone returns the done channel, which will be closed when all runtimes have terminated.
func (g *gronos[K]) OnDone() <-chan struct{} {
	return g.done
}

func (g *gronos[K]) readMessages(errChan chan<- error) {
	// drain the channel until it is closed
	for m := range g.com {
		if err := g.handleMessage(m); err != nil {
			errChan <- err
		}
	}
}

func (g *gronos[K]) automaticShutdown() {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	defer func() {
		log.Info("[Gronos] closing askShutting")
		close(g.askShutting)
		close(g.shutting) // to trigger the other goroutine that need to stop
	}()

	// check if we need to shutdown
	for {
		select {
		// asked for immediate shutdown
		case <-g.askShutting:
			log.Info("[Gronos] automatic shutdown askShutting triggered")
			g.shutdownAllAndWait()
			ticker.Stop()
			return
		case <-ticker.C:
			if g.isShutting.Load() {
				log.Info("[Gronos] ticker shutdown stop", "reason", "isShutting")
				// we do not need automatic shutdown anymore
				ticker.Stop()
				continue
			}
			log.Info("[Gronos] ticker shutdown")
			if g.config.shutdownBehavior == ShutdownAutomatic {
				log.Info("[Gronos] check automatic shutdown")
				if g.checkAutomaticShutdown() {
					return
				}
			}
		}
		runtime.Gosched() // give cpu time to other goroutines
	}
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
		log.Info("[Gronos] run closed")
	}()

	// blocking until all applications are done
	select {
	case <-g.shutting:
		log.Info("[Gronos] run shutting down")
		return
	case <-g.ctx.Done():
		log.Info("[Gronos] context done")
		g.com <- MsgAllCancelShutdown[K]()
		return
	}
}

// focused on shutting down all applications and waiting for them to finish
func (g *gronos[K]) shutdownAllAndWait() {
	log.Info("[Gronos] Initiating shutdownAllAndWait")

	timeout := time.After(10 * time.Second) // Set the total timeout duration
	log.Info("[Gronos] Timeout set for 10 seconds")

	shutdownComplete := false

	for !shutdownComplete {
		select {
		case <-timeout:
			// Exit the loop if the total timeout duration has passed
			log.Error("[Gronos] Shutdown timed out")
			g.errChan <- fmt.Errorf("shutdown timed out")
			shutdownComplete = true
		default:
			// Send shutdown message
			log.Info("[Gronos] Sending MsgAllShutdown")
			g.com <- MsgAllShutdown[K]()

			// Wait for a specified timeout
			log.Info("[Gronos] Waiting for 1 second")
			<-time.After(1 * time.Second)

			// Check if there are any alive instances
			log.Info("[Gronos] Checking if any instances are alive")
			doneAlive, msgAlive := MsgRequestAllAlive[K]()
			g.com <- msgAlive
			if alive := <-doneAlive; !alive {
				log.Info("[Gronos] No instances are alive")
				shutdownComplete = true
			} else {
				log.Info("[Gronos] Some instances are still alive")
			}
		}
	}

	log.Info("[Gronos] Entering grace period")
	<-time.After(g.config.gracePeriod)
	log.Info("[Gronos] Shutdown all and finished waiting")
}

// check if all applications are alive or not, if not then time to gronos shutdown
func (g *gronos[K]) checkAutomaticShutdown() bool {
	if time.Since(g.startTime) < g.config.minRuntime {
		return false
	}

	log.Info("[Gronos] check automatic shutdown sending all shutdown")

	doneAlive, msgAlive := MsgRequestAllAlive[K]()
	g.com <- msgAlive
	log.Info("[Gronos] Sent MsgRequestAllAlive")
	alive := <-doneAlive
	log.Info("[Gronos] Received MsgRequestAllAlive", "alive", alive)

	if alive {
		log.Info("[Gronos] Instances might be still alive")
		return false
	}

	g.shutdownAllAndWait()

	return true
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
	state := g.GetStatus(k)
	return (state == StatusRunning)
}

// IsComplete checks if a component has completed
func (g *gronos[K]) IsComplete(k K) bool {
	state := g.GetStatus(k)
	return (state == StatusShutdownCancelled ||
		state == StatusShutdownError ||
		state == StatusShutdownTerminated ||
		state == StatusShutdownPanicked)
}

// GetStatus retrieves the current status of a component
func (g *gronos[K]) GetStatus(k K) StatusState {
	done, msg := MsgRequestStatus(k)
	g.com <- msg
	return <-done
}

type addOptions struct {
	whenState StatusState
}

type addOption func(*addOptions)

func WhenState(state StatusState) addOption {
	return func(o *addOptions) {
		o.whenState = state
	}
}

// Add adds a new application to the gronos instance with the given key and RuntimeApplication.
func (g *gronos[K]) Add(k K, v RuntimeApplication, opts ...addOption) <-chan struct{} {
	cfg := addOptions{
		whenState: StatusAdded,
	}
	for _, opt := range opts {
		opt(&cfg)
	}

	// Add new runtime application
	g.com <- MsgAddRuntimeApplication[K](k, v)

	// Request asynchronous confirmation of the state
	done, msg := MsgRequestStatusAsync[K](k, cfg.whenState)

	// send the message
	g.com <- msg

	proxy := make(chan struct{})
	go func() {
		select {
		case <-time.After(5 * time.Second):
			log.Printf("Timeout waiting for application %v to reach state %v", k, cfg.whenState)
		case <-done:
			log.Printf("Application %v reached state %v", k, cfg.whenState)
		}
		close(proxy)
	}()

	// return receive only channel if the user want to listen when the runtime application is added
	return proxy
}

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

// func UseBus[K comparable](ctx context.Context) (chan<- Message, error) {
// 	comValue := ctx.Value(comKey)
// 	if comValue == nil {
// 		return nil, fmt.Errorf("com not found in context")
// 	}

// 	keyValue := ctx.Value(keyKey)
// 	if keyValue == nil {
// 		return nil, fmt.Errorf("key not found in context")
// 	}

// 	comChan := comValue.(chan Message)

// 	proxyChan := make(chan Message)

// 	go func() {
// 		for msg := range proxyChan {
// 			comChan <- Envelope[K]{
// 				From:    keyValue.(K),
// 				Message: msg,
// 			}
// 		}
// 	}()

// 	return proxyChan, nil
// }

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
