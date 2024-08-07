// Package gronos provides a concurrent application management system.
package gronos

import (
	"context"
	"errors"
	"fmt"
	"reflect"
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
	ErrUnhandledMessage         = errors.New("unhandled message")
	ErrUnmanageExtensionMessage = errors.New("unmanage extension message")
	ErrPanic                    = errors.New("panic")
)

type ShutdownKind string

const (
	ShutdownKindTerminate ShutdownKind = "terminate"
	ShutdownKindCancel    ShutdownKind = "cancel"
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
	com chan Message

	// main waiting group for all applications
	// wait sync.WaitGroup

	ctx        context.Context
	cancel     context.CancelFunc
	config     gronosConfig
	startTime  time.Time
	started    atomic.Bool // define if gronos started or not
	extensions []ExtensionHooks[K]
	errChan    chan error

	// when we are shutting down to prevent triggering multiple shutdowns
	isShutting atomic.Bool

	// init map is only used for the initial applications and restarts
	init map[K]RuntimeApplication

	shutdownChan chan struct{}
	doneChan     chan struct{}
	comClosed    atomic.Bool
}

// gronos instance doesn't know about the state, you have to request it
type gronosState[K comparable] struct {
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

	automaticShutdown atomic.Bool
	shutting          atomic.Bool
}

type Option[K comparable] func(*gronos[K])

func WithExtension[K comparable](ext ExtensionHooks[K]) Option[K] {
	return func(ctx *gronos[K]) {
		ctx.extensions = append(ctx.extensions, ext)
	}
}

// New creates a new gronos instance with the given context and initial applications.
func New[K comparable](ctx context.Context, init map[K]RuntimeApplication, opts ...Option[K]) (*gronos[K], chan error) {

	log.Default().SetLevel(log.DebugLevel) // debug

	ctx, cancel := context.WithCancel(ctx)
	g := &gronos[K]{
		com:    make(chan Message, 500),
		cancel: cancel,
		// Context will be monitored to detect cancellation and trigger ForceCancelShutdown
		ctx: ctx,
		// Shutdown channel will be monitored to detect shutdown and trigger ForceTerminateShutdown
		shutdownChan: make(chan struct{}),
		// Once shutdown process is complete, done channel will be closed
		doneChan: make(chan struct{}),

		errChan:    make(chan error, 100),
		extensions: []ExtensionHooks[K]{},
		config: gronosConfig{
			// TODO: make it configurable
			// TODO: make a sub shutdown struct
			shutdownBehavior: ShutdownManual,
			gracePeriod:      2 * time.Second,
			minRuntime:       2 * time.Second,
		},
		init: init,
	}
	for _, opt := range opts {
		opt(g)
	}
	return g, g.Start()
}

func (g *gronos[K]) reinitialize() {
	g.shutdownChan = make(chan struct{})
	g.doneChan = make(chan struct{})
	g.com = make(chan Message, 200)
	g.errChan = make(chan error, 100)
	g.isShutting.Store(false)
	g.started.Store(false)
	// Reset any other necessary fields
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
		g.reinitialize()
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

	// really starting it so we can process messages from here
	go g.run(g.errChan)

	// TODO: might send them all at once and wait for all of them to be added
	for k, v := range g.init {
		wait, msg := MsgAddRuntimeApplication[K](k, v)
		g.sendMessage(msg)
		<-wait
	}

	// Start automatic shutdown if configured
	if g.config.shutdownBehavior == ShutdownAutomatic {
		go g.automaticShutdown()
	}

	return g.errChan
}

// Shutdown initiates the shutdown process for all applications managed by the gronos instance.
func (g *gronos[K]) Shutdown() {
	if !g.isShutting.CompareAndSwap(false, true) {
		log.Debug("[Gronos] Shutdown already in progress")
		return
	}
	log.Debug("[Gronos] Initiating shutdown")
	close(g.shutdownChan)
}

// Wait blocks until all applications managed by the gronos instance have terminated.
func (g *gronos[K]) Wait() {
	defer close(g.errChan) // at the very very end
	log.Debug("[Gronos] wait", g.doneChan)
	if !g.started.Load() {
		// wasn't even started
		log.Debug("[Gronos] wasn't started")
		return
	}

	_, ok := <-g.doneChan
	log.Debug("[Gronos] wait done", ok)
}

// OnDone returns the done channel, which will be closed when all runtimes have terminated.
func (g *gronos[K]) OnDone() <-chan struct{} {
	return g.doneChan
}

func (g *gronos[K]) sendMessage(m Message) bool {
	if !g.comClosed.Load() {
		select {
		case g.com <- m:
			return true
		default:
			log.Debug("[Gronos] Unable to send message, channel might be full")
			return false
		}
	}
	return false
}

// if configured on automatic shutdown, it will check the status of the applications
func (g *gronos[K]) automaticShutdown() {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-g.ctx.Done():
			log.Debug("[Gronos] Context cancelled, stopping automatic shutdown")
			return
		case <-ticker.C:
			if g.isShutting.Load() {
				log.Debug("[Gronos] Shutdown in progress, stopping automatic shutdown")
				return
			}
			done, msg := MsgCheckAutomaticShutdown[K]()
			g.sendMessage(msg)
			<-done
		}
		runtime.Gosched() // give CPU time to other goroutines
	}
}

// run is the main loop of the gronos instance, handling messages and managing applications.
func (g *gronos[K]) run(errChan chan<- error) {

	log.Debug("[Gronos] Entering run method")
	defer log.Debug("[Gronos] Exiting run method")

	defer func() {
		// Apply extensions' OnStop hooks
		for _, ext := range g.extensions {
			if err := ext.OnStop(g.ctx, errChan); err != nil {
				errChan <- fmt.Errorf("extension error on stop: %w", err)
			}
		}
		g.sendMessage(MsgDestroy[K]())
	}()

	state := &gronosState[K]{}

	// global shutdown or cancellation detection
	go func() {
		select {
		case <-g.ctx.Done():
			log.Debug("[Gronos] Context cancelled, initiating shutdown")
			g.sendMessage(MsgInitiateContextCancellation[K]())
		case <-g.shutdownChan:
			log.Debug("[Gronos] Shutdown initiated, initiating shutdown")
			g.sendMessage(MsgInitiateShutdown[K]())
		}
	}()

	for m := range g.com {
		fmt.Println("message", m, reflect.TypeOf(m))
		if err := g.handleMessage(state, m); err != nil {
			errChan <- err
		}
	}
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
	g.sendMessage(msg)
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

	done, msgAdd := MsgAddRuntimeApplication(k, v)

	// Add new runtime application
	if !g.sendMessage(msgAdd) {
		log.Debug("[Gronos] Unable to add runtime application")
		return nil
	}

	select {
	case <-done:
		// Application added successfully
	case <-time.After(5 * time.Second):
		log.Debug("[Gronos] Timeout waiting for application to be added")
		g.sendMessage(MsgRuntimeError(k, fmt.Errorf("timeout waiting for application to be added")))
		return nil
	}

	// Request asynchronous confirmation of the state
	doneStatus, msgStatus := MsgRequestStatusAsync(k, cfg.whenState)

	// send the message
	if !g.sendMessage(msgStatus) {
		log.Debug("[Gronos] Unable to request status")
		return nil
	}

	proxy := make(chan struct{})
	go func() {
		select {
		case <-time.After(5 * time.Second):
			log.Printf("Timeout waiting for application %v to reach state %v", k, cfg.whenState)
		case <-doneStatus:
			log.Printf("Application %v reached state %v", k, cfg.whenState)
		}
		close(proxy)
	}()

	// return receive only channel if the user want to listen when the runtime application is added
	return proxy
}

// createContext creates a new context with the gronos communication channel embedded.
func (g *gronos[K]) createContext() (context.Context, context.CancelFunc) {
	ctx := context.WithValue(context.Background(), comKey, g.sendMessage)
	ctx, cancel := context.WithCancel(ctx)
	return ctx, cancel
}

// UseBus retrieves the communication channel from a context created by gronos.
func UseBus(ctx context.Context) (func(m Message) bool, error) {
	value := ctx.Value(comKey)
	if value == nil {
		return nil, fmt.Errorf("com not found in context")
	}
	return value.(func(m Message) bool), nil
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
