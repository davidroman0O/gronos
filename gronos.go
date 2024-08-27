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
	"github.com/heimdalr/dag"
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

// LifecyleFunc is a function type representing an application that can be run concurrently.
// It takes a context and a shutdown channel as parameters and returns an error.
type LifecyleFunc func(ctx context.Context, shutdown <-chan struct{}) error

type gronosConfig struct {
	shutdownBehavior ShutdownBehavior
	gracePeriod      time.Duration
	immediatePeriod  time.Duration
	minRuntime       time.Duration
	wait             bool
}

type MessagePayload[K comparable] struct {
	*Metadata[K]
	Message
}

var messagePayloadPool sync.Pool
var runtimeApplicationIncrement = 0

func newIncrement() int {
	runtimeApplicationIncrement++
	return runtimeApplicationIncrement
}

var metadataPool sync.Pool

// gronos is the main struct that manages concurrent applications.
// It is parameterized by a comparable key type K.
type gronos[K comparable] struct {
	com chan *MessagePayload[K]

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
	init map[K]LifecyleFunc

	shutdownChan chan struct{}
	doneChan     chan struct{}
	comClosed    atomic.Bool

	computedRootKey K
	hasRootKey      atomic.Bool
}

type LifecycleVertexData[K comparable] struct {
	Key       interface{}
	cachedKey string
}

func NewLifecycleVertexData[K comparable](key K) *LifecycleVertexData[K] {
	return &LifecycleVertexData[K]{Key: key}
}

// ID returns the unique identifier of the node
func (n *LifecycleVertexData[K]) ID() string {
	if n.cachedKey != "" {
		return n.cachedKey
	}
	switch v := n.Key.(type) {
	case string:
		n.cachedKey = v
	default:
		n.cachedKey = fmt.Sprintf("%v", n.Key)
	}
	if len(n.cachedKey) == 0 {
		n.cachedKey = fmt.Sprintf("node-%p", n.Key)
	}
	return n.cachedKey
}

// Metadata returns the metadata of the node
func (n *LifecycleVertexData[K]) Metadata() map[string]string {
	return nil
}

// SetMetadata sets the metadata of the node
func (n *LifecycleVertexData[K]) SetMetadata(metadata map[string]string) {

}

// gronos instance doesn't know about the state, you have to request it for goroutine safety
type gronosState[K comparable] struct {
	rootKey    K
	rootVertex string

	// We need to know dynamically the structure of the application
	graph *dag.DAG

	// it add a slight overhead in memory but faster access
	// we mostly do reads so it's fine to have sync.Map

	mkeys   *GMap[K, K]
	mapp    *GMap[K, LifecyleFunc]            // application func - key is mkey
	mctx    *GMap[K, context.Context]         // ctx - key is mkey
	mcom    *GMap[K, chan *MessagePayload[K]] // com chan - key is mkey
	mret    *GMap[K, uint]                    // retries - key is mkey
	mshu    *GMap[K, chan struct{}]           // shutdown chan - key is mkey
	mali    *GMap[K, bool]                    // alive - key is mkey
	mrea    *GMap[K, error]                   // reason - key is mkey
	mcloser *GMap[K, func()]                  // closer - key is mkey
	mcancel *GMap[K, func()]                  // cancel - key is mkey
	mstatus *GMap[K, StatusState]             // status - key is mkey
	mdone   *GMap[K, chan struct{}]           // done - key is mkey

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

func Merge[K comparable](apps ...map[K]LifecyleFunc) map[K]LifecyleFunc {
	m := make(map[K]LifecyleFunc)
	for _, app := range apps {
		for k, v := range app {
			m[k] = v
		}
	}
	return m
}

func WithRootKey[K comparable](key K) Option[K] {
	return func(ctx *gronos[K]) {
		ctx.computedRootKey = key
		ctx.hasRootKey.Store(true)
	}
}

// New creates a new gronos instance with the given context and initial applications.
func New[K comparable](ctx context.Context, init map[K]LifecyleFunc, opts ...Option[K]) (*gronos[K], chan error) {

	// log.Default().SetLevel(log.DebugLevel) // debug

	ctx, cancel := context.WithCancel(ctx)
	g := &gronos[K]{
		init: init,

		com:    make(chan *MessagePayload[K], 500),
		cancel: cancel,
		// Context will be monitored to detect cancellation and trigger ForceCancelShutdown
		ctx: ctx,
		// Shutdown channel will be monitored to detect shutdown and trigger ForceTerminateShutdown
		shutdownChan: make(chan struct{}),
		// Once shutdown process is complete, done channel will be closed
		doneChan: make(chan struct{}),

		errChan: make(chan error, 100),

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
	}
	for _, opt := range opts {
		opt(g)
	}

	if !g.hasRootKey.Load() {
		g.computedRootKey = g.getRootKey()
		g.hasRootKey.Store(true)
	}

	// dynamically
	metadataPool = sync.Pool{
		New: func() interface{} {
			return &Metadata[K]{}
		},
	}

	messagePayloadPool = sync.Pool{
		New: func() interface{} {
			return &MessagePayload[K]{
				Metadata: nil,
				Message:  nil,
			}
		},
	}

	return g, g.Start()
}

func (g *gronos[K]) reinitialize() {
	g.shutdownChan = make(chan struct{})
	g.doneChan = make(chan struct{})
	g.com = make(chan *MessagePayload[K], 200)
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
		g.sendMessage(g.getSystemMetadata(), msg)
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

}

// OnDone returns the done channel, which will be closed when all runtimes have terminated.
func (g *gronos[K]) OnDone() <-chan struct{} {
	return g.doneChan
}

// We don't want to infringe on the memory space of the user
// It will be Put back when the message is processed
func (g *gronos[K]) poolMessagePayload(metadata *Metadata[K], m Message) *MessagePayload[K] {
	payload := messagePayloadPool.Get()
	msgPayload := payload.(*MessagePayload[K])
	msgPayload.Metadata = metadata

	// it's always pointers normally
	typeOf := reflect.TypeOf(m)

	msgPayload.Metadata.SetType(typeOf)

	if typeOf.Kind() == reflect.Ptr {
		msgPayload.Metadata.SetName(fmt.Sprintf("%s.%s", typeOf.Elem().PkgPath(), typeOf.Elem().Name()))
	} else {
		msgPayload.Metadata.SetName(fmt.Sprintf("%s.%s", typeOf.PkgPath(), typeOf.Name()))
		msgPayload.Metadata.SetError(fmt.Errorf("it should be a pointer"))
	}

	// every messages that users are sending will be pre-analyzed
	if _, loaded := g.typeMapping.LoadOrStore(msgPayload.Metadata.GetName(), typeOf); !loaded {
		// TODO: send an event for metrics for "new message type" detected
	}
	msgPayload.Message = m
	return msgPayload
}

func (g *gronos[K]) poolMetadata() *Metadata[K] {
	return metadataPool.Get().(*Metadata[K])
}

func (g *gronos[K]) getSystemMetadata() *Metadata[K] {
	metadata := g.poolMetadata()
	metadata.SetID(0)
	metadata.SetKey(g.computedRootKey)
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

func (g *gronos[K]) sendMessage(metadata *Metadata[K], m Message) bool {
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

func (g *gronos[K]) sendMessageWait(metadata *Metadata[K], fn FnWait) <-chan struct{} {
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

func (g *gronos[K]) sendMessageConfirm(metadata *Metadata[K], fn FnConfirm) <-chan bool {
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
	ticker := time.NewTicker(time.Second / 2)
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

func (g *gronos[K]) getRootKey() K {
	typeOf := reflect.TypeFor[K]()
	var rootKey K
	if fmt.Sprintf("%v", rootKey) == "" {
		switch typeOf.Kind() {
		case reflect.String:
			rootKey = reflect.ValueOf("root").Interface().(K)
		case reflect.Int:
			rootKey = reflect.ValueOf(1).Interface().(K)
		case reflect.Int64:
			rootKey = reflect.ValueOf(int64(1)).Interface().(K)
		case reflect.Int32:
			rootKey = reflect.ValueOf(int32(1)).Interface().(K)
		case reflect.Int16:
			rootKey = reflect.ValueOf(int16(1)).Interface().(K)
		case reflect.Int8:
			rootKey = reflect.ValueOf(int8(1)).Interface().(K)
		case reflect.Uint:
			rootKey = reflect.ValueOf(uint(1)).Interface().(K)
		case reflect.Uint64:
			rootKey = reflect.ValueOf(uint64(1)).Interface().(K)
		case reflect.Uint32:
			rootKey = reflect.ValueOf(uint32(1)).Interface().(K)
		case reflect.Uint16:
			rootKey = reflect.ValueOf(uint16(1)).Interface().(K)
		case reflect.Uint8:
			rootKey = reflect.ValueOf(uint8(1)).Interface().(K)
		case reflect.Float64:
			rootKey = reflect.ValueOf(float64(1)).Interface().(K)
		case reflect.Float32:
			rootKey = reflect.ValueOf(float32(1)).Interface().(K)
		case reflect.Bool:
			rootKey = reflect.ValueOf(false).Interface().(K)
		default:
			var inter interface{} = rootKey
			switch inter.(type) {
			case fmt.Stringer:
				rootKey = reflect.ValueOf("root").Interface().(K)
			default: // really default default
				rootKey = reflect.Zero(typeOf).Interface().(K)
			}
		}
	}
	return rootKey
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

	dag := dag.NewDAG()

	state := &gronosState[K]{
		// dag with weights
		graph:   dag,
		mkeys:   &GMap[K, K]{},
		mapp:    &GMap[K, LifecyleFunc]{},
		mctx:    &GMap[K, context.Context]{},
		mcom:    &GMap[K, chan *MessagePayload[K]]{},
		mret:    &GMap[K, uint]{},
		mshu:    &GMap[K, chan struct{}]{},
		mali:    &GMap[K, bool]{},
		mrea:    &GMap[K, error]{},
		mcloser: &GMap[K, func()]{},
		mcancel: &GMap[K, func()]{},
		mstatus: &GMap[K, StatusState]{},
		mdone:   &GMap[K, chan struct{}]{},
	}

	// Prepare the graph
	state.rootKey = g.getRootKey()

	var err error
	if state.rootVertex, err = state.graph.AddVertex(NewLifecycleVertexData(state.rootKey)); err != nil {
		errChan <- fmt.Errorf("error adding root vertex: %w", err)
		return
	}

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
	log.Debug("[Gronos] Communication channel closed")
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

func (g *gronos[K]) Push(apps map[K]LifecyleFunc, opts ...addOption) <-chan struct{} {
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
func (g *gronos[K]) Add(k K, v LifecyleFunc, opts ...addOption) <-chan struct{} {
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

// createContext creates a new context with the gronos communication channel embedded.
func (g *gronos[K]) createContext(key K) (context.Context, context.CancelFunc) {
	ctx := context.WithValue(context.Background(), keyID, newIncrement())

	ctx = context.WithValue(ctx, keyKey, key)

	ctx = context.WithValue(ctx, comKey, func(m Message) bool {
		metadataAny := metadataPool.Get()
		metadata := metadataAny.(*Metadata[K])
		metadata.SetID(ctx.Value(keyID).(int))
		metadata.SetKey(ctx.Value(keyKey).(K))
		return g.sendMessage(metadata, m)
	})

	ctx = context.WithValue(ctx, comKeyWait, func(fn FnWait) <-chan struct{} {
		metadataAny := metadataPool.Get()
		metadata := metadataAny.(*Metadata[K])
		metadata.SetID(ctx.Value(keyID).(int))
		metadata.SetKey(ctx.Value(keyKey).(K))
		return g.sendMessageWait(metadata, fn)
	})

	ctx = context.WithValue(ctx, comKeyConfirm, func(fn FnConfirm) <-chan bool {
		metadataAny := metadataPool.Get()
		metadata := metadataAny.(*Metadata[K])
		metadata.SetID(ctx.Value(keyID).(int))
		metadata.SetKey(ctx.Value(keyKey).(K))
		return g.sendMessageConfirm(metadata, fn)
	})
	ctx, cancel := context.WithCancel(ctx)
	return ctx, cancel
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
