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
var comKeyWait ctxKey = "comwait"
var comKeyConfirm ctxKey = "comconfirm"
var keyID ctxKey = "id"
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
/// - AddMessage: Transitions from Added to Running.
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
	StatusNotFound           StatusState = "shutdown_not_found"
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
	immediatePeriod  time.Duration
	minRuntime       time.Duration
	wait             bool
}

type MessagePayload struct {
	Metadata map[string]interface{}
	Message
}

var messagePayloadPool sync.Pool = sync.Pool{
	New: func() interface{} {
		return &MessagePayload{
			Metadata: make(map[string]interface{}),
			Message:  nil,
		}
	},
}

// gronos is the main struct that manages concurrent applications.
// It is parameterized by a comparable key type K.
type gronos[K comparable] struct {
	com chan *MessagePayload

	// main waiting group for all applications
	// wait sync.WaitGroup

	ctx        context.Context
	cancel     context.CancelFunc
	config     gronosConfig
	startTime  time.Time
	started    atomic.Bool // define if gronos started or not
	extensions []Extension[K]
	errChan    chan error

	typeMapping sync.Map

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

	wait              sync.WaitGroup
	automaticShutdown atomic.Bool
	shutting          atomic.Bool
}

type Option[K comparable] func(*gronos[K])

func WithExtension[K comparable](ext Extension[K]) Option[K] {
	return func(ctx *gronos[K]) {
		ctx.extensions = append(ctx.extensions, ext)
	}
}

func Merge[K comparable](apps ...map[K]RuntimeApplication) map[K]RuntimeApplication {
	m := make(map[K]RuntimeApplication)
	for _, app := range apps {
		for k, v := range app {
			m[k] = v
		}
	}
	return m
}

// New creates a new gronos instance with the given context and initial applications.
func New[K comparable](ctx context.Context, init map[K]RuntimeApplication, opts ...Option[K]) (*gronos[K], chan error) {

	// log.Default().SetLevel(log.DebugLevel) // debug

	ctx, cancel := context.WithCancel(ctx)
	g := &gronos[K]{
		com:    make(chan *MessagePayload, 500),
		cancel: cancel,
		// Context will be monitored to detect cancellation and trigger ForceCancelShutdown
		ctx: ctx,
		// Shutdown channel will be monitored to detect shutdown and trigger ForceTerminateShutdown
		shutdownChan: make(chan struct{}),
		// Once shutdown process is complete, done channel will be closed
		doneChan: make(chan struct{}),

		errChan:    make(chan error, 100),
		extensions: []Extension[K]{},
		config: gronosConfig{
			shutdownBehavior: ShutdownManual,
			// We do not guarantee the delay we will wait for your RuntimeApplication to shutdown
			// We have a grace period for which we will wait before unleaching the immediate period timer
			gracePeriod: 10 * time.Millisecond,
			// If you don't shutdown in the grace period, we will wait for the immediate period, then immediately panic them all
			immediatePeriod: 10 * time.Millisecond,
			// In automatic shutdown, we will wait AT LEAST this duration before allowing the shutdown
			minRuntime: 0,
			// By default, we won't wait for your RuntimeApplication to shutdown, but you can allow to still force gronos to wait
			// But after the immediatePeriod + gracePeriod, it will still panic (except if you set it to zero)!
			wait: false,
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
	g.com = make(chan *MessagePayload, 200)
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
		wait, msg := MsgAdd[K](k, v)
		g.sendMessage(map[string]interface{}{}, msg)
		<-wait
	}

	// Start automatic shutdown if configured
	if g.config.shutdownBehavior == ShutdownAutomatic {
		go g.automaticShutdown()
	}

	return g.errChan
}

// Shutdown initiates the shutdown process for all applications managed by the gronos instance.
func (g *gronos[K]) Shutdown() bool {
	if time.Since(g.startTime) < g.config.minRuntime {
		log.Debug("[Gronos] Runtime is less than minRuntime, delaying shutdown")
		return false
	}
	if !g.isShutting.CompareAndSwap(false, true) {
		log.Debug("[Gronos] Shutdown already in progress")
		return false
	}
	log.Debug("[Gronos] Initiating shutdown")
	close(g.shutdownChan)
	return true
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
	<-time.After(time.Second / 5)
}

// OnDone returns the done channel, which will be closed when all runtimes have terminated.
func (g *gronos[K]) OnDone() <-chan struct{} {
	return g.doneChan
}

// We don't want to infringe on the memory space of the user
// It will be Put back when the message is processed
func (g *gronos[K]) poolMessagePayload(metadata map[string]interface{}, m Message) *MessagePayload {
	payload := messagePayloadPool.Get()
	msgPayload := payload.(*MessagePayload)
	msgPayload.Metadata = metadata
	// it's always pointers normally
	typeOf := reflect.TypeOf(m)
	msgPayload.Metadata["$type"] = typeOf
	if typeOf.Kind() == reflect.Ptr {
		msgPayload.Metadata["$name"] = fmt.Sprintf("%s.%s", typeOf.Elem().PkgPath(), typeOf.Elem().Name())
	} else {
		msgPayload.Metadata["$name"] = fmt.Sprintf("%s.%s", typeOf.PkgPath(), typeOf.Name())
		msgPayload.Metadata["$error"] = "it should be a pointer"
	}
	// every messages that users are sending will be pre-analyzed
	if _, loaded := g.typeMapping.LoadOrStore(msgPayload.Metadata["$name"], typeOf); !loaded {
		// TODO: send an event for metrics for "new message type" detected
	}
	msgPayload.Message = m
	return msgPayload
}

func (g *gronos[K]) poolMetadata() map[string]interface{} {
	return metadataPool.Get().(map[string]interface{})
}

func (g *gronos[K]) getSystemMetadata() map[string]interface{} {
	metadata := g.poolMetadata()
	metadata["$id"] = 0
	metadata["$key"] = "system"
	return metadata
}

func (g *gronos[K]) Send(m Message) bool {
	return g.sendMessage(g.getSystemMetadata(), m)
}

func (g *gronos[K]) WaitFor(fn FnWait) <-chan struct{} {
	return g.sendMessageWait(g.getSystemMetadata(), fn)
}

func (g *gronos[K]) Confirm(fn FnConfirm) <-chan bool {
	return g.sendMessageConfirm(g.getSystemMetadata(), fn)
}

func (g *gronos[K]) sendMessage(metadata map[string]interface{}, m Message) bool {
	if !g.comClosed.Load() {
		select {
		case g.com <- g.poolMessagePayload(metadata, m):
			return true
		default:
			log.Debug("[Gronos] Unable to send message, channel might be full")
			return false
		}
	}
	return false
}

type FnWait func() (<-chan struct{}, Message)

func (g *gronos[K]) sendMessageWait(metadata map[string]interface{}, fn FnWait) <-chan struct{} {
	// fn is supposed to be a function that returns a `<-chan struct` and `message`
	// execute the function and return the channel and message
	done, msg := fn()

	if !g.comClosed.Load() {
		select {
		case g.com <- g.poolMessagePayload(metadata, msg):
			return done
		default:
			log.Debug("[Gronos] Unable to send message, channel might be full")
			return done
		}
	}
	return done
}

type FnConfirm func() (<-chan bool, Message)

func (g *gronos[K]) sendMessageConfirm(metadata map[string]interface{}, fn FnConfirm) <-chan bool {
	// fn is supposed to be a function that returns a `<-chan struct` and `message`
	// execute the function and return the channel and message
	done, msg := fn()

	if !g.comClosed.Load() {
		select {
		case g.com <- g.poolMessagePayload(metadata, msg):
			return done
		default:
			log.Debug("[Gronos] Unable to send message, channel might be full")
			return done
		}
	}
	return done
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
			g.sendMessage(g.getSystemMetadata(), msg)
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
		g.sendMessage(g.getSystemMetadata(), MsgDestroy[K]())
	}()

	state := &gronosState[K]{}
	g.startTime = time.Now()

	// global shutdown or cancellation detection
	go func() {
		select {
		case <-g.ctx.Done():
			log.Debug("[Gronos] Context cancelled, initiating shutdown")
			g.sendMessage(g.getSystemMetadata(), MsgInitiateContextCancellation[K]())
		case <-g.shutdownChan:
			log.Debug("[Gronos] Shutdown initiated, initiating shutdown")
			g.sendMessage(g.getSystemMetadata(), MsgInitiateShutdown[K]())
		}
	}()

	for m := range g.com {
		if err := g.handleMessage(state, m); err != nil {
			errChan <- err
		}
	}
	log.Warn("[Gronos] Communication channel closed")
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

func (g *gronos[K]) IsMissing(k K) bool {
	state := g.GetStatus(k)
	return (state == StatusNotFound)
}

// GetStatus retrieves the current status of a component
func (g *gronos[K]) GetStatus(k K) StatusState {
	done, msg := MsgRequestStatus(k)
	g.sendMessage(g.getSystemMetadata(), msg)
	return <-done
}

func (g *gronos[K]) GetList() ([]K, error) {
	done, msg := MsgGetListRuntimeApplication[K]()
	g.sendMessage(g.getSystemMetadata(), msg)
	return <-done, nil
}

// ShutdownAndWait initiates shutdown for specified applications and waits for their completion.
// It returns a map of application keys to their final status.
func (g *gronos[K]) ShutdownAndWait(keys ...K) map[K]StatusState {
	results := make(map[K]StatusState)
	var wg sync.WaitGroup

	for _, key := range keys {
		wg.Add(1)
		go func(k K) {
			defer wg.Done()

			// Request current status
			status := g.GetStatus(k)

			// If the application is not already in a final state, initiate shutdown
			if status != StatusShutdownCancelled &&
				status != StatusShutdownError &&
				status != StatusShutdownTerminated &&
				status != StatusShutdownPanicked {

				// Initiate terminate shutdown
				_, msg := MsgForceTerminateShutdown(k)
				g.sendMessage(g.getSystemMetadata(), msg)

				// Wait for the application to reach a final state
				for {
					status = g.GetStatus(k)
					if status == StatusShutdownCancelled ||
						status == StatusShutdownError ||
						status == StatusShutdownTerminated ||
						status == StatusShutdownPanicked {
						break
					}
					time.Sleep(100 * time.Millisecond)
				}
			}

			results[k] = status
		}(key)
	}

	wg.Wait()
	return results
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

func (g *gronos[K]) Push(apps map[K]RuntimeApplication, opts ...addOption) <-chan struct{} {
	done := make(chan struct{})
	go func() {
		defer close(done)
		acc := []<-chan struct{}{}
		for k, v := range apps {
			acc = append(acc, g.Add(k, v, opts...))
		}
		for _, d := range acc {
			<-d
		}
		acc = nil
	}()
	return done
}

// Add adds a new application to the gronos instance with the given key and RuntimeApplication.
func (g *gronos[K]) Add(k K, v RuntimeApplication, opts ...addOption) <-chan struct{} {
	cfg := addOptions{
		whenState: StatusAdded,
	}
	for _, opt := range opts {
		opt(&cfg)
	}

	done, msgAdd := MsgAdd(k, v)

	metadata := g.getSystemMetadata()

	// Add new runtime application
	if !g.sendMessage(metadata, msgAdd) {
		log.Debug("[Gronos] Unable to add runtime application")
		return nil
	}

	select {
	case <-done:
		// Application added successfully
	case <-time.After(5 * time.Second):
		log.Debug("[Gronos] Timeout waiting for application to be added")
		g.sendMessage(metadata, MsgRuntimeError(k, fmt.Errorf("timeout waiting for application to be added")))
		return nil
	}

	// Request asynchronous confirmation of the state
	doneStatus, msgStatus := MsgRequestStatusAsync(k, cfg.whenState)

	// send the message
	if !g.sendMessage(metadata, msgStatus) {
		log.Debug("[Gronos] Unable to request status")
		return nil
	}

	proxy := make(chan struct{})
	go func() {
		select {
		case <-time.After(5 * time.Second):
			log.Debug("Timeout waiting for application to reach state", k, cfg.whenState)
		case <-doneStatus:
			log.Debug("Application reached state", k, cfg.whenState)
		}
		close(proxy)
	}()

	// return receive only channel if the user want to listen when the runtime application is added
	return proxy
}

var runtimeApplicationIncrement = 0

func newIncrement() int {
	runtimeApplicationIncrement++
	return runtimeApplicationIncrement
}

var metadataPool sync.Pool = sync.Pool{
	New: func() interface{} {
		return make(map[string]interface{})
	},
}

// createContext creates a new context with the gronos communication channel embedded.
func (g *gronos[K]) createContext(key K) (context.Context, context.CancelFunc) {
	ctx := context.WithValue(context.Background(), keyID, newIncrement())

	ctx = context.WithValue(ctx, keyKey, key)

	ctx = context.WithValue(ctx, comKey, func(m Message) bool {
		metadataAny := metadataPool.New()
		metadata := metadataAny.(map[string]interface{})
		metadata["$id"] = ctx.Value(keyID).(int)
		metadata["$key"] = ctx.Value(keyKey)
		return g.sendMessage(metadata, m)
	})

	ctx = context.WithValue(ctx, comKeyWait, func(fn FnWait) <-chan struct{} {
		metadataAny := metadataPool.New()
		metadata := metadataAny.(map[string]interface{})
		metadata["$id"] = ctx.Value(keyID).(int)
		metadata["$key"] = ctx.Value(keyKey)
		return g.sendMessageWait(metadata, fn)
	})

	ctx = context.WithValue(ctx, comKeyConfirm, func(fn FnConfirm) <-chan bool {
		metadataAny := metadataPool.New()
		metadata := metadataAny.(map[string]interface{})
		metadata["$id"] = ctx.Value(keyID).(int)
		metadata["$key"] = ctx.Value(keyKey)
		return g.sendMessageConfirm(metadata, fn)
	})
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

func UseBusWait(ctx context.Context) (func(fn FnWait) <-chan struct{}, error) {
	value := ctx.Value(comKeyWait)
	if value == nil {
		return nil, fmt.Errorf("com not found in context")
	}
	return value.(func(fn FnWait) <-chan struct{}), nil
}

func UseBusConfirm(ctx context.Context) (func(fn FnConfirm) <-chan bool, error) {
	value := ctx.Value(comKeyConfirm)
	if value == nil {
		return nil, fmt.Errorf("com not found in context")
	}
	return value.(func(fn FnConfirm) <-chan bool), nil
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

func WithImmediatePeriod[K comparable](period time.Duration) Option[K] {
	return func(g *gronos[K]) {
		g.config.immediatePeriod = period
	}
}
func WithWait[K comparable]() Option[K] {
	return func(g *gronos[K]) {
		g.config.wait = true
	}
}

func WithMinRuntime[K comparable](duration time.Duration) Option[K] {
	return func(g *gronos[K]) {
		g.config.minRuntime = duration
	}
}

func WithoutImmediatePeriod[K comparable]() Option[K] {
	return func(g *gronos[K]) {
		g.config.immediatePeriod = 0
	}
}

func WithoutGracePeriod[K comparable]() Option[K] {
	return func(g *gronos[K]) {
		g.config.gracePeriod = 0
	}
}

func WithoutMinRuntime[K comparable]() Option[K] {
	return func(g *gronos[K]) {
		g.config.minRuntime = 0
	}
}
