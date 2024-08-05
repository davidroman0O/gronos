package gronos

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"runtime"
	"sync"
	"time"

	"github.com/avast/retry-go/v3"
	"github.com/charmbracelet/log"
)

/// Each message is a set of properties that will mutate one by one the state of the system

// Message is an interface type for internal communication within gronos.
type Message interface{}

type Envelope[K comparable] struct {
	From K
	Message
}

// Composable header
type KeyMessage[K comparable] struct {
	Key K
}

// Used for generic requests
type RequestMessage[K comparable, Y any] struct {
	KeyMessage[K]
	Response chan Y
}

/// Lifecycle messages

type AddRuntimeApplicationMessage[K comparable] struct {
	KeyMessage[K]
	RuntimeApplication
}

type CancelShutdown[K comparable] struct {
	KeyMessage[K]
	Error error
}

type TerminateShutdown[K comparable] struct {
	KeyMessage[K]
}

type PanicShutdown[K comparable] struct {
	KeyMessage[K]
	Error error
}

type ErrorShutdown[K comparable] struct {
	KeyMessage[K]
	Error error
}

type AllShutdown[K comparable] struct{}

type AllCancelShutdown[K comparable] struct{}

/// Request-Response messages

type RequestStatus[K comparable] struct {
	KeyMessage[K]
	RequestMessage[K, StatusState]
}

type RequestAlive[K comparable] struct {
	KeyMessage[K]
	RequestMessage[K, bool]
}

type RequestReason[K comparable] struct {
	KeyMessage[K]
	RequestMessage[K, error]
}

type RequestAllAlive[K comparable] struct {
	RequestMessage[K, bool]
}

type RequestStatusAsync[K comparable] struct {
	KeyMessage[K]
	When StatusState
	RequestMessage[K, struct{}]
}

/// Functions request-response messages

func MsgAddRuntimeApplication[K comparable](key K, app RuntimeApplication) *AddRuntimeApplicationMessage[K] {
	msg := addRuntimeApplicationPool.Get().(*AddRuntimeApplicationMessage[K])
	msg.Key = key
	msg.RuntimeApplication = app
	return msg
}

func MsgCancelShutdown[K comparable](key K, err error) *CancelShutdown[K] {
	msg := cancelShutdownPool.Get().(*CancelShutdown[K])
	msg.Key = key
	msg.Error = err
	return msg
}

func MsgTerminateShutdown[K comparable](key K) *TerminateShutdown[K] {
	msg := terminateShutdownPool.Get().(*TerminateShutdown[K])
	msg.Key = key
	return msg
}

func MsgPanicShutdown[K comparable](key K, err error) *PanicShutdown[K] {
	msg := panicShutdownPool.Get().(*PanicShutdown[K])
	msg.Key = key
	msg.Error = err
	return msg
}

func MsgErrorShutdown[K comparable](key K, err error) *ErrorShutdown[K] {
	msg := errorShutdownPool.Get().(*ErrorShutdown[K])
	msg.Key = key
	msg.Error = err
	return msg
}

func MsgAllShutdown[K comparable]() *AllShutdown[K] {
	return allShutdownPool.Get().(*AllShutdown[K])
}

func MsgAllCancelShutdown[K comparable]() *AllCancelShutdown[K] {
	return allCancelShutdownPool.Get().(*AllCancelShutdown[K])
}

func MsgRequestStatus[K comparable](key K) (<-chan StatusState, *RequestStatus[K]) {
	response := make(chan StatusState, 1)
	msg := requestStatusPool.Get().(*RequestStatus[K])
	msg.Key = key
	msg.Response = response
	return response, msg
}

func MsgRequestAlive[K comparable](key K) *RequestAlive[K] {
	msg := requestAlivePool.Get().(*RequestAlive[K])
	msg.Key = key
	msg.Response = make(chan bool, 1)
	return msg
}

func MsgRequestReason[K comparable](key K) *RequestReason[K] {
	msg := requestReasonPool.Get().(*RequestReason[K])
	msg.Key = key
	msg.Response = make(chan error, 1)
	return msg
}

func MsgRequestAllAlive[K comparable]() (<-chan bool, *RequestAllAlive[K]) {
	response := make(chan bool, 1)
	msg := requestAllAlivePool.Get().(*RequestAllAlive[K])
	msg.Response = response
	return response, msg
}

func MsgRequestStatusAsync[K comparable](key K, when StatusState) (<-chan struct{}, *RequestStatusAsync[K]) {
	response := make(chan struct{}, 1)
	msg := requestStatusAsyncPool.Get().(*RequestStatusAsync[K])
	msg.Key = key
	msg.When = when
	msg.Response = response
	return response, msg
}

/// Static pools
/// I don't care what some people say about having this here, I will have a fixed amount of messages that Gronos manage, it won't be dynamic
/// You are responsible for the messaging systems, i'm just trying to avoid too much overhead

var addRuntimeApplicationPool = sync.Pool{
	New: func() any {
		return &AddRuntimeApplicationMessage[string]{}
	},
}

var cancelShutdownPool = sync.Pool{
	New: func() any {
		return &CancelShutdown[string]{}
	},
}

var terminateShutdownPool = sync.Pool{
	New: func() any {
		return &TerminateShutdown[string]{}
	},
}

var panicShutdownPool = sync.Pool{
	New: func() any {
		return &PanicShutdown[string]{}
	},
}

var errorShutdownPool = sync.Pool{
	New: func() any {
		return &ErrorShutdown[string]{}
	},
}

var allShutdownPool = sync.Pool{
	New: func() any {
		return &AllShutdown[string]{}
	},
}

var allCancelShutdownPool = sync.Pool{
	New: func() any {
		return &AllCancelShutdown[string]{}
	},
}

var requestStatusPool = sync.Pool{
	New: func() any {
		return &RequestStatus[string]{}
	},
}

var requestAlivePool = sync.Pool{
	New: func() any {
		return &RequestAlive[string]{}
	},
}

var requestReasonPool = sync.Pool{
	New: func() any {
		return &RequestReason[string]{}
	},
}

var requestAllAlivePool = sync.Pool{
	New: func() any {
		return &RequestAllAlive[string]{}
	},
}

var requestStatusAsyncPool = sync.Pool{
	New: func() any {
		return &RequestStatusAsync[string]{}
	},
}

func (g *gronos[K]) handleGronosMessage(m Message) error {
	log.Info("[GronosMessage] handle gronos message", reflect.TypeOf(m).Name(), m)
	switch msg := m.(type) {
	case *AddRuntimeApplicationMessage[K]:
		log.Info("[GronosMessage] [AddRuntimeApplicationMessage]", msg.Key)
		defer addRuntimeApplicationPool.Put(msg)
		return g.handleAddRuntimeApplication(msg)
	case *CancelShutdown[K]:
		log.Info("[GronosMessage] [CancelShutdown]", msg.Key)
		defer cancelShutdownPool.Put(msg)
		return g.handleCancelShutdown(msg)
	case *AllShutdown[K]:
		log.Info("[GronosMessage] [AllShutdown]")
		defer allShutdownPool.Put(msg)
		return g.handleAllShutdown(msg)
	case *TerminateShutdown[K]:
		log.Info("[GronosMessage] [TerminateShutdown]", msg.Key)
		defer terminateShutdownPool.Put(msg)
		return g.handleTerminateShutdown(msg)
	case *RequestStatus[K]:
		log.Info("[GronosMessage] [RequestStatus]", msg.Key)
		defer requestStatusPool.Put(msg)
		return g.handleRequestStatus(msg)
	case *RequestAlive[K]:
		log.Info("[GronosMessage] [RequestAlive]", msg.Key)
		defer requestAlivePool.Put(msg)
		return g.handleRequestAlive(msg)
	case *RequestReason[K]:
		log.Info("[GronosMessage] [RequestReason]", msg.Key)
		defer requestReasonPool.Put(msg)
		return g.handleRequestReason(msg)
	case *RequestAllAlive[K]:
		log.Info("[GronosMessage] [RequestAllAlive]")
		defer requestAllAlivePool.Put(msg)
		return g.handleRequestAllAlive(msg)
	case *RequestStatusAsync[K]:
		log.Info("[GronosMessage] [RequestStatusAsync]", msg.Key)
		defer requestStatusAsyncPool.Put(msg)
		return g.handleRequestStatusAsync(msg)
	default:
		return ErrUnhandledMessage
	}
}

func (g *gronos[K]) handleAddRuntimeApplication(msg *AddRuntimeApplicationMessage[K]) error {
	var ok bool

	log.Info("[GronosMessage] [AddRuntimeApplicationMessage] add application", msg.Key)

	if _, ok = g.mkeys.Load(msg.Key); ok {
		return fmt.Errorf("application with key %v already exists", msg.Key)
	}
	if g.ctx.Err() != nil {
		return fmt.Errorf("context is already cancelled")
	}
	if g.isShutting.Load() {
		return fmt.Errorf("gronos is shutting down")
	}

	ctx, cancel := g.createContext()
	shutdown := make(chan struct{})

	log.Info("[GronosMessage] [AddRuntimeApplicationMessage] add application with extensions", msg.Key, msg.RuntimeApplication)
	// extend context with new eventual context
	for _, ext := range g.extensions {
		ctx = ext.OnNewRuntime(ctx)
	}

	g.mkeys.Store(msg.Key, msg.Key)
	g.mapp.Store(msg.Key, msg.RuntimeApplication)
	g.mctx.Store(msg.Key, ctx)
	g.mcom.Store(msg.Key, g.com)
	g.mshu.Store(msg.Key, shutdown)
	g.mali.Store(msg.Key, true)
	g.mrea.Store(msg.Key, nil)
	g.mstatus.Store(msg.Key, StatusAdded)

	var retries uint = 0
	g.mret.Store(msg.Key, retries)

	realDone := make(chan struct{})
	g.mdone.Store(msg.Key, realDone)

	g.mcloser.Store(msg.Key, sync.OnceFunc(func() {
		close(shutdown)
		<-realDone
	}))

	g.mcancel.Store(msg.Key, sync.OnceFunc(func() {
		cancel()   // cancel first
		<-realDone // wait for runtime to be finished
		// then close the shutdown channel as it is useless
		if v, ok := g.mcloser.Load(msg.Key); ok {
			fn := v.(func())
			fn()
		}
	}))

	g.wait.Add(1)

	go g.handleRuntimeApplication(msg.Key, g.com)
	return nil
}

func (g *gronos[K]) handleCancelShutdown(msg *CancelShutdown[K]) error {
	var value any
	var ok bool
	if value, ok = g.mali.Load(msg.Key); !ok {
		return fmt.Errorf("app not found (alive property) %v", msg.Key)
	}
	if !value.(bool) {
		return fmt.Errorf("app already dead %v", msg.Key)
	}

	//	set application to unalive
	if value.(bool) {
		g.mali.Store(msg.Key, false)
	}

	// get cancel function
	if value, ok = g.mcancel.Load(msg.Key); !ok {
		return fmt.Errorf("app not found (closer property) %v", msg.Key)
	}

	//	notifying it is shuting down
	g.mstatus.Store(msg.Key, StatusShutingDown)

	// trigger closer
	value.(func())() // the shutdown channel is now closed, runtime application is currently shutting down

	if value, ok = g.mdone.Load(msg.Key); !ok {
		return fmt.Errorf("app not found (done property) %v", msg.Key)
	}

	// wait until the shutdown is REALLY done
	<-value.(chan struct{})

	//	notifying it is really cancelled
	g.mstatus.Store(msg.Key, StatusShutdownCancelled)

	g.wait.Done()

	return nil
}

func (g *gronos[K]) handleAllShutdown(msg *AllShutdown[K]) error {
	var ok bool
	var value any

	g.mkeys.Range(func(key, v any) bool {
		log.Info("[GronosMessage] [AllShutdown] all shutdown", key)
		if value, ok = g.mali.Load(key); !ok {
			return false
		}
		log.Info("[GronosMessage] [AllShutdown] all shutdown alive", key, value.(bool))
		if value.(bool) {
			log.Info("[GronosMessage] [AllShutdown] all shutdown send terminate", key)
			g.com <- MsgTerminateShutdown(key.(K))
		}
		return true
	})

	return nil
}

func (g *gronos[K]) handleTerminateShutdown(msg *TerminateShutdown[K]) error {
	var value any
	var ok bool
	if value, ok = g.mali.Load(msg.Key); !ok {
		return fmt.Errorf("app not found (alive property) %v", msg.Key)
	}
	if !value.(bool) {
		return fmt.Errorf("app already dead %v", msg.Key)
	}

	//	set application to unalive
	if value.(bool) {
		g.mali.Store(msg.Key, false)
	}

	// get closer function
	if value, ok = g.mcloser.Load(msg.Key); !ok {
		return fmt.Errorf("app not found (closer property) %v", msg.Key)
	}

	//	notifying it is shuting down
	g.mstatus.Store(msg.Key, StatusShutingDown)

	// trigger closer from `mcloser`
	value.(func())() // the shutdown channel is now closed, runtime application is currently shutting down

	if value, ok = g.mdone.Load(msg.Key); !ok {
		return fmt.Errorf("app not found (done property) %v", msg.Key)
	}

	log.Info("[GronosMessage] [TerminateShutdown] terminate shutdown waiting real done", msg.Key)
	// wait until the shutdown is REALLY done from `mdone`
	_, ok = <-value.(chan struct{})
	log.Info("[GronosMessage] [TerminateShutdown] terminate shutdown done", msg.Key, ok)
	// notifying it is really terminated
	g.mstatus.Store(msg.Key, StatusShutdownTerminated)

	g.wait.Done()

	log.Info("[GronosMessage] [TerminateShutdown] terminate shutdown terminated", msg.Key)

	return nil
}

func (g *gronos[K]) handleRequestStatus(msg *RequestStatus[K]) error {
	var value any
	var ok bool
	if value, ok = g.mstatus.Load(msg.Key); !ok {
		return fmt.Errorf("app not found (status property) %v", msg.Key)
	}
	msg.Response <- value.(StatusState)
	close(msg.Response)
	return nil
}

func (g *gronos[K]) handleRequestAlive(msg *RequestAlive[K]) error {
	var value any
	var ok bool
	if value, ok = g.mali.Load(msg.Key); !ok {
		return fmt.Errorf("app not found (alive property) %v", msg.Key)
	}
	msg.Response <- value.(bool)
	close(msg.Response)
	return nil
}

func (g *gronos[K]) handleRequestReason(msg *RequestReason[K]) error {
	var value any
	var ok bool
	if value, ok = g.mrea.Load(msg.Key); !ok {
		return fmt.Errorf("app not found (reason property) %v", msg.Key)
	}
	msg.Response <- value.(error)
	close(msg.Response)
	return nil
}

func (g *gronos[K]) handleRequestAllAlive(msg *RequestAllAlive[K]) error {
	var alive bool
	g.mali.Range(func(key, value any) bool {
		if value.(bool) {
			alive = true
			return false
		}
		return true
	})
	msg.Response <- alive
	close(msg.Response)
	log.Info("[GronosMessage] all alive", alive)
	return nil
}

func (g *gronos[K]) handleRequestStatusAsync(msg *RequestStatusAsync[K]) error {
	go func() {
		var currentState int
		for currentState < stateNumber(msg.When) {
			if value, ok := g.mstatus.Load(msg.Key); ok {
				currentState = stateNumber(value.(StatusState))
			}
			<-time.After(time.Second / 16)
			runtime.Gosched()
		}
		log.Info("[GronosMessage] status async", msg.Key, currentState)
		close(msg.Response)
	}()
	return nil
}

func (g *gronos[K]) handleRuntimeApplication(key K, com chan Message) {
	var retries uint
	var shutdown chan struct{}
	var app RuntimeApplication
	var ctx context.Context

	// Load necessary data
	if value, ok := g.mret.Load(key); ok {
		retries = value.(uint)
	}
	if value, ok := g.mapp.Load(key); ok {
		app = value.(RuntimeApplication)
	}
	if value, ok := g.mctx.Load(key); ok {
		ctx = value.(context.Context)
	}
	if value, ok := g.mshu.Load(key); ok {
		shutdown = value.(chan struct{})
	}

	ctx = context.WithValue(ctx, keyKey, key)

	g.mstatus.Store(key, StatusRunning)

	log.Info("[RuntimeApplication] goroutine executed", key)

	errChan := make(chan error, 1)
	done := make(chan struct{})

	go func() {
		defer close(done)
		log.Info("[RuntimeApplication] goroutine start", key)
		defer func() {
			if r := recover(); r != nil {
				var err error
				switch v := r.(type) {
				case error:
					err = errors.Join(v, ErrPanic)
				case string:
					err = errors.Join(errors.New(v), ErrPanic)
				default:
					err = errors.Join(fmt.Errorf("%v", v), ErrPanic)
				}
				errChan <- err
				close(shutdown)
			}
		}()

		var err error
		if retries == 0 {
			log.Info("[RuntimeApplication] goroutine start runtime application", key)
			err = app(ctx, shutdown)
		} else {
			log.Info("[RuntimeApplication] goroutine start retriable runtime application", key)
			err = retry.Do(func() error {
				return app(ctx, shutdown)
			}, retry.Attempts(retries))
		}
		errChan <- err
		log.Info("[RuntimeApplication] goroutine done", key, err)
	}()

	log.Info("[RuntimeApplication] waiting goroutine", key)

	var err error
	select {
	case <-ctx.Done():
		err = ctx.Err()
	case <-shutdown:
	case <-done:
	}

	// Check for any error from the goroutine
	select {
	case goroutineErr := <-errChan:
		if goroutineErr != nil {
			err = goroutineErr
		}
	default:
	}

	log.Info("[RuntimeApplication] wait done", key, err)

	defer func() {
		log.Info("[RuntimeApplication] defer", key, err)
		if value, ok := g.mdone.Load(key); ok {
			close(value.(chan struct{}))
		} else {
			log.Info("[RuntimeApplication] defer not found", key)
		}
		log.Info("[RuntimeApplication] defer done", key)
	}()

	// Extend context cleanup
	for _, ext := range g.extensions {
		ctx = ext.OnStopRuntime(ctx)
	}

	// Check if the application is still alive
	if value, ok := g.mali.Load(key); !ok || !value.(bool) {
		return
	}

	log.Info("[RuntimeApplication] com", key, err)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			log.Info("[RuntimeApplication] com canceled", key, err)
			com <- MsgCancelShutdown(key, err)
		} else if errors.Is(err, ErrPanic) {
			log.Info("[RuntimeApplication] com panic", key, err)
			com <- MsgPanicShutdown(key, err)
		} else {
			log.Info("[RuntimeApplication] com error", key, err)
			com <- MsgErrorShutdown(key, err)
		}
	} else {
		log.Info("[RuntimeApplication] com terminate", key)
		com <- MsgTerminateShutdown(key)
	}
}
