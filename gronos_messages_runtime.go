package gronos

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/avast/retry-go/v3"
	"github.com/charmbracelet/log"
)

type AddMessage[K comparable] struct {
	KeyMessage[K]
	RuntimeApplication
	RequestMessage[K, struct{}]
}

type RemoveMessage[K comparable] struct {
	KeyMessage[K]
	RequestMessage[K, bool]
}

type RuntimeError[K comparable] struct {
	KeyMessage[K]
	Error error
}

func MsgRuntimeError[K comparable](key K, err error) *RuntimeError[K] {
	return &RuntimeError[K]{KeyMessage[K]{key}, err}
}

// global system force cancel
type ForceCancelShutdown[K comparable] struct {
	KeyMessage[K]
	Error error
	RequestMessage[K, struct{}]
}

// global system force terminate
type ForceTerminateShutdown[K comparable] struct {
	KeyMessage[K]
	RequestMessage[K, struct{}]
}

type CancelledShutdown[K comparable] struct {
	KeyMessage[K]
	Error error
	RequestMessage[K, struct{}]
}

type TerminatedShutdown[K comparable] struct {
	RequestMessage[K, struct{}]
}

type PanickedShutdown[K comparable] struct {
	KeyMessage[K]
	Error error
	RequestMessage[K, struct{}]
}

type ErroredShutdown[K comparable] struct {
	KeyMessage[K]
	Error error
	RequestMessage[K, struct{}]
}

type GetListRuntimeApplication[K comparable] struct {
	RequestMessage[K, []K]
}

var addRuntimeApplicationPoolInited bool = false
var addRuntimeApplicationPool sync.Pool

var removeRuntimeApplicationPoolInited bool = false
var removeRuntimeApplicationPool sync.Pool

var cancelShutdownPoolInited bool = false
var cancelShutdownPool sync.Pool

var terminatedShutdownPoolInited bool = false
var terminatedShutdownPool sync.Pool

var forceCancelShutdownPoolInited bool = false
var forceCancelShutdownPool sync.Pool

var forceTerminateShutdownPoolInited bool = false
var forceTerminateShutdownPool sync.Pool

var panicShutdownPoolInited bool = false
var panickedShutdownPool sync.Pool

var erroredShutdownPoolInited bool = false
var erroredShutdownPool sync.Pool

var getListRuntimeApplicationPoolInited bool = false
var getListRuntimeApplicationPool sync.Pool

func MsgGetListRuntimeApplication[K comparable]() (<-chan []K, *GetListRuntimeApplication[K]) {
	if !getListRuntimeApplicationPoolInited {
		getListRuntimeApplicationPoolInited = true
		getListRuntimeApplicationPool = sync.Pool{
			New: func() any {
				return &GetListRuntimeApplication[K]{}
			},
		}
	}
	msg := getListRuntimeApplicationPool.Get().(*GetListRuntimeApplication[K])
	msg.Response = make(chan []K, 1)
	return msg.Response, msg
}

func MsgAdd[K comparable](key K, app RuntimeApplication) (<-chan struct{}, *AddMessage[K]) {
	if !addRuntimeApplicationPoolInited {
		addRuntimeApplicationPoolInited = true
		addRuntimeApplicationPool = sync.Pool{
			New: func() any {
				return &AddMessage[K]{}
			},
		}
	}
	msg := addRuntimeApplicationPool.Get().(*AddMessage[K])
	msg.Key = key
	msg.RuntimeApplication = app
	msg.Response = make(chan struct{}, 1)
	return msg.Response, msg
}

func MsgRemove[K comparable](key K) (<-chan bool, *RemoveMessage[K]) {
	if !removeRuntimeApplicationPoolInited {
		removeRuntimeApplicationPoolInited = true
		removeRuntimeApplicationPool = sync.Pool{
			New: func() any {
				return &RemoveMessage[K]{}
			},
		}
	}
	msg := removeRuntimeApplicationPool.Get().(*RemoveMessage[K])
	msg.Key = key
	msg.Response = make(chan bool, 1)
	return msg.Response, msg
}

func MsgForceCancelShutdown[K comparable](key K, err error) (<-chan struct{}, *ForceCancelShutdown[K]) {
	if !forceCancelShutdownPoolInited {
		forceCancelShutdownPoolInited = true
		forceCancelShutdownPool = sync.Pool{
			New: func() any {
				return &ForceCancelShutdown[K]{}
			},
		}
	}
	msg := forceCancelShutdownPool.Get().(*ForceCancelShutdown[K])
	msg.Key = key
	msg.Error = err
	msg.Response = make(chan struct{}, 1)
	return msg.Response, msg
}

func MsgForceTerminateShutdown[K comparable](key K) (<-chan struct{}, *ForceTerminateShutdown[K]) {
	if !forceTerminateShutdownPoolInited {
		forceTerminateShutdownPoolInited = true
		forceTerminateShutdownPool = sync.Pool{
			New: func() any {
				return &ForceTerminateShutdown[K]{}
			},
		}
	}
	msg := forceTerminateShutdownPool.Get().(*ForceTerminateShutdown[K])
	msg.Key = key
	msg.Response = make(chan struct{}, 1)
	return msg.Response, msg
}

func msgCancelledShutdown[K comparable](key K, err error) (<-chan struct{}, *CancelledShutdown[K]) {
	if !cancelShutdownPoolInited {
		cancelShutdownPoolInited = true
		cancelShutdownPool = sync.Pool{
			New: func() any {
				return &CancelledShutdown[K]{}
			},
		}
	}
	msg := cancelShutdownPool.Get().(*CancelledShutdown[K])
	msg.Key = key
	msg.Error = err
	response := make(chan struct{}, 1)
	msg.Response = response
	return response, msg
}

func msgTerminatedShutdown[K comparable](key K) (<-chan struct{}, *TerminatedShutdown[K]) {
	if !terminatedShutdownPoolInited {
		terminatedShutdownPoolInited = true
		terminatedShutdownPool = sync.Pool{
			New: func() any {
				return &TerminatedShutdown[K]{}
			},
		}
	}
	msg := terminatedShutdownPool.Get().(*TerminatedShutdown[K])
	msg.Key = key
	response := make(chan struct{}, 1)
	msg.Response = response
	return response, msg
}

func msgPanickedShutdown[K comparable](key K, err error) (<-chan struct{}, *PanickedShutdown[K]) {
	if !panicShutdownPoolInited {
		panicShutdownPoolInited = true
		panickedShutdownPool = sync.Pool{
			New: func() any {
				return &PanickedShutdown[K]{}
			},
		}
	}
	msg := panickedShutdownPool.Get().(*PanickedShutdown[K])
	msg.Key = key
	msg.Error = err
	response := make(chan struct{}, 1)
	msg.Response = response
	return response, msg
}

func msgErroredShutdown[K comparable](key K, err error) (<-chan struct{}, *ErroredShutdown[K]) {
	if !erroredShutdownPoolInited {
		erroredShutdownPoolInited = true
		erroredShutdownPool = sync.Pool{
			New: func() any {
				return &ErroredShutdown[K]{}
			},
		}
	}
	msg := erroredShutdownPool.Get().(*ErroredShutdown[K])
	msg.Key = key
	msg.Error = err
	response := make(chan struct{}, 1)
	msg.Response = response
	return response, msg
}

func (g *gronos[K]) handleRuntimeApplicationMessage(state *gronosState[K], m *MessagePayload) (error, bool) {
	switch msg := m.Message.(type) {
	case *AddMessage[K]:
		log.Debug("[GronosMessage] [AddMessage]", msg.Key)
		defer addRuntimeApplicationPool.Put(msg)
		return g.handleAddRuntimeApplication(state, msg.Key, msg.Response, msg.RuntimeApplication), true
	case *RemoveMessage[K]:
		log.Debug("[GronosMessage] [RemoveMessage]", msg.Key)
		defer removeRuntimeApplicationPool.Put(msg)
		return g.handleRemoveRuntimeApplication(state, msg.Key, m.Metadata, msg.Response), true
	case *CancelledShutdown[K]:
		log.Debug("[GronosMessage] [CancelShutdown]", msg.Key)
		defer cancelShutdownPool.Put(msg)
		return g.handleCancelledShutdown(state, msg.Key, msg.Error, msg.Response), true
	case *TerminatedShutdown[K]:
		log.Debug("[GronosMessage] [TerminateShutdown]", msg.Key)
		defer terminatedShutdownPool.Put(msg)
		return g.handleTerminateShutdown(state, msg.Key, msg.Response), true
	case *PanickedShutdown[K]:
		log.Debug("[GronosMessage] [PanicShutdown]")
		defer panickedShutdownPool.Put(msg)
		return g.handlePanicShutdown(state, msg.Key, msg.Error), true
	case *ErroredShutdown[K]:
		log.Debug("[GronosMessage] [ErrorShutdown]")
		defer erroredShutdownPool.Put(msg)
		return g.handleErrorShutdown(state, msg.Key, msg.Error), true
	case *ForceCancelShutdown[K]:
		log.Debug("[GronosMessage] [ForceCancelShutdown]")
		defer forceCancelShutdownPool.Put(msg)
		return g.handleForceCancelShutdown(state, msg.Key, msg.Error), true
	case *ForceTerminateShutdown[K]:
		defer forceTerminateShutdownPool.Put(msg)
		log.Debug("[GronosMessage] [ForceTerminateShutdown]")
		return g.handleForceTerminateShutdown(state, msg.Key, msg.Response), true
	case *GetListRuntimeApplication[K]:
		log.Debug("[GronosMessage] [GetListRuntimeApplication]")
		defer getListRuntimeApplicationPool.Put(msg)
		return g.handleRequestListRuntimeApplication(state, msg.Response), true
	default:
		return nil, false
	}
}

func (g *gronos[K]) handleRequestListRuntimeApplication(state *gronosState[K], response chan []K) error {
	defer close(response)
	var list []K
	state.mkeys.Range(func(k, v any) bool {
		list = append(list, k.(K))
		return true
	})
	response <- list
	return nil
}

// need to check if the have it or not, terminate it and remove it
// it's an async process
func (g *gronos[K]) handleRemoveRuntimeApplication(state *gronosState[K], key K, metadata map[string]interface{}, done chan bool) error {

	if state.shutting.Load() {
		return fmt.Errorf("gronos is shutting down")
	}

	remove := func() {
		state.mkeys.Delete(key)
		state.mapp.Delete(key)
		state.mctx.Delete(key)
		state.mcom.Delete(key)
		state.mshu.Delete(key)
		state.mali.Delete(key)
		state.mrea.Delete(key)
		state.mstatus.Delete(key)
		state.mret.Delete(key)
		state.mdone.Delete(key)
		state.mcloser.Delete(key)
		state.mcancel.Delete(key)
		log.Debug("[GronosMessage] [RemoveMessage] application removed", key)
		done <- true
		close(done)
	}

	log.Debug("[GronosMessage] [RemoveMessage] remove application", key)

	var value any
	var ok bool
	if value, ok = state.mali.Load(key); !ok {
		log.Debug("[GronosMessage] [RemoveMessage] malive application not found", key) // but that's fine
		remove()
		return nil
	}
	if !value.(bool) {
		log.Debug("[GronosMessage] [RemoveMessage] application already dead", key) // but that's fine
		remove()
		return nil
	} else {
		// then it will be async
		go func(whenTerminated <-chan struct{}) {
			log.Debug("[GronosMessage] [RemoveMessage] terminate application", key)
			<-whenTerminated
			remove()
		}(
			g.sendMessageWait(metadata, func() (<-chan struct{}, Message) {
				return MsgForceTerminateShutdown(key)
			}),
		)
		return nil
	}
}

func (g *gronos[K]) handleAddRuntimeApplication(state *gronosState[K], key K, done chan struct{}, app RuntimeApplication) error {
	defer close(done)

	if state.shutting.Load() {
		return fmt.Errorf("gronos is shutting down")
	}

	log.Debug("[GronosMessage] [AddMessage] add application", key)

	if _, ok := state.mkeys.Load(key); ok {
		return fmt.Errorf("application with key %v already exists", key)
	}
	if g.ctx.Err() != nil {
		return g.ctx.Err()
	}
	if g.isShutting.Load() {
		return fmt.Errorf("gronos is shutting down")
	}

	ctx, cancel := g.createContext(key)
	shutdown := make(chan struct{})

	log.Debug("[GronosMessage] [AddMessage] add application with extensions", key, app)
	for _, ext := range g.extensions {
		ctx = ext.OnNewRuntime(ctx)
	}

	state.mkeys.Store(key, key)
	state.mapp.Store(key, app)
	state.mctx.Store(key, ctx)
	state.mcom.Store(key, g.com)
	state.mshu.Store(key, shutdown)
	state.mali.Store(key, true)
	state.mrea.Store(key, nil)
	state.mstatus.Store(key, StatusAdded)

	var retries uint = 0
	state.mret.Store(key, retries)

	realDone := make(chan struct{})
	state.mdone.Store(key, realDone)

	state.mcloser.Store(key, sync.OnceFunc(func() {
		log.Debug("[GronosMessage] [AddMessage] close", key)
		close(shutdown)
	}))

	state.mcancel.Store(key, sync.OnceFunc(func() {
		log.Debug("[GronosMessage] [AddMessage] cancel", key)
		cancel()
	}))

	go g.handleRuntimeApplication(state, key, g.sendMessage)

	log.Debug("[GronosMessage] [AddMessage] application added", key)

	return nil
}

func (g *gronos[K]) handleForceCancelShutdown(state *gronosState[K], key K, err error) error {
	var value any
	var ok bool
	if value, ok = state.mali.Load(key); !ok {
		log.Debug("[GronosMessage] [ForceCancelShutdown] app not found (alive property)", key)
		return fmt.Errorf("app not found (alive property) %v", key)
	}
	if !value.(bool) {
		log.Debug("[GronosMessage] [ForceCancelShutdown] app already dead", key)
		return fmt.Errorf("app already dead %v", key)
	}

	log.Debug("[GronosMessage] [ForceCancelShutdown] cancel", key, err)
	if value, ok = state.mcancel.Load(key); !ok {
		log.Debug("[GronosMessage] [ForceCancelShutdown] app not found (cancel property)", key)
		return fmt.Errorf("app not found (closer property) %v", key)
	}

	state.mstatus.Store(key, StatusShutingDown)

	value.(func())()

	state.mstatus.Store(key, StatusShutingDown)
	log.Debug("[GronosMessage] [ForceCancelShutdown] cancel done", key)

	return nil
}

func (g *gronos[K]) handleForceTerminateShutdown(state *gronosState[K], key K, response chan struct{}) error {
	defer close(response)
	var value any
	var ok bool
	if value, ok = state.mali.Load(key); !ok {
		return fmt.Errorf("app not found (alive property) %v", key)
	}
	if !value.(bool) {
		return fmt.Errorf("app already dead %v", key)
	}

	log.Debug("[GronosMessage] [ForceTerminateShutdown] terminate shutdown", key)
	if value, ok = state.mcloser.Load(key); !ok {
		return fmt.Errorf("app not found (closer property) %v", key)
	}

	value.(func())()

	state.mstatus.Store(key, StatusShutingDown)

	log.Debug("[GronosMessage] [ForceTerminateShutdown] terminate shutdown done", key)

	return nil
}

func (g *gronos[K]) handleCancelledShutdown(state *gronosState[K], key K, err error, response chan struct{}) error {
	var value any
	var ok bool
	if value, ok = state.mali.Load(key); !ok {
		return fmt.Errorf("app not found (alive property) %v", key)
	}
	if !value.(bool) {
		return fmt.Errorf("app already dead %v", key)
	}

	if value, ok = state.mdone.Load(key); !ok {
		return fmt.Errorf("app not found (done property) %v", key)
	}

	log.Debug("[GronosMessage] [CancelledShutdown] terminate cancelled shutdown waiting real done", key)

	go func() {

		metadata := g.getSystemMetadata()

		<-value.(chan struct{})
		log.Debug("[GronosMessage] [CancelledShutdown] terminate cancelled shutdown done", key, ok)

		if value, ok = state.mali.Load(key); !ok {
			g.sendMessage(metadata, MsgRuntimeError(key, fmt.Errorf("app not found (alive property) %v", key)))
			return
		}

		if value.(bool) {
			state.mali.Store(key, false)
		}

		g.sendMessage(metadata, MsgRuntimeError(key, err))
		close(response)

		state.mstatus.Store(key, StatusShutdownCancelled)

		log.Debug("[GronosMessage] [CancelledShutdown] terminate cancelled shutdown terminated", key)

	}()

	return nil
}

func (g *gronos[K]) handleTerminateShutdown(state *gronosState[K], key K, response chan struct{}) error {
	var value any
	var ok bool
	if value, ok = state.mali.Load(key); !ok {
		return fmt.Errorf("app not found (alive property) %v", key)
	}
	if !value.(bool) {
		return fmt.Errorf("app already dead %v", key)
	}

	if value, ok = state.mdone.Load(key); !ok {
		return fmt.Errorf("app not found (done property) %v", key)
	}

	log.Debug("[GronosMessage] [TerminateShutdown] terminate shutdown waiting real done", key)

	go func() {

		metadata := g.getSystemMetadata()
		<-value.(chan struct{})
		log.Debug("[GronosMessage] [TerminateShutdown] terminate shutdown done", key, ok)

		if value, ok = state.mali.Load(key); !ok {
			g.sendMessage(metadata, MsgRuntimeError(key, fmt.Errorf("app not found (alive property) %v", key)))
			return
		}

		if value.(bool) {
			state.mali.Store(key, false)
		}

		state.mstatus.Store(key, StatusShutdownTerminated)

		log.Debug("[GronosMessage] [TerminateShutdown] terminate shutdown terminated", key)

		close(response)
	}()

	return nil
}

func (g *gronos[K]) handlePanicShutdown(state *gronosState[K], key K, err error) error {
	var value any
	var ok bool
	if value, ok = state.mali.Load(key); !ok {
		return fmt.Errorf("app not found (alive property) %v", key)
	}
	if !value.(bool) {
		return fmt.Errorf("app already dead %v", key)
	}

	log.Debug("[GronosMessage] [PanickedShutdown] panic", key, err)
	state.mrea.Store(key, err)
	state.mstatus.Store(key, StatusShutdownPanicked)
	if value.(bool) {
		state.mali.Store(key, false)
	}

	metadata := g.getSystemMetadata()
	g.sendMessage(metadata, MsgRuntimeError(key, err))

	return nil
}

func (g *gronos[K]) handleErrorShutdown(state *gronosState[K], key K, err error) error {
	var value any
	var ok bool
	if value, ok = state.mali.Load(key); !ok {
		return fmt.Errorf("app not found (alive property) %v", key)
	}
	if !value.(bool) {
		return fmt.Errorf("app already dead %v", key)
	}

	log.Debug("[GronosMessage] [ErroredShutdown] error", key, err)
	state.mrea.Store(key, err)
	state.mstatus.Store(key, StatusShutdownError)
	if value.(bool) {
		state.mali.Store(key, false)
	}

	metadata := g.getSystemMetadata()
	g.sendMessage(metadata, MsgRuntimeError(key, err))

	return nil
}

func (g *gronos[K]) handleRuntimeApplication(state *gronosState[K], key K, send func(metadata map[string]interface{}, m Message) bool) {
	var retries uint
	var shutdown chan struct{}
	var app RuntimeApplication
	var ctx context.Context

	// Load necessary data
	if value, ok := state.mret.Load(key); ok {
		retries = value.(uint)
	}
	if value, ok := state.mapp.Load(key); ok {
		app = value.(RuntimeApplication)
	}
	if value, ok := state.mctx.Load(key); ok {
		ctx = value.(context.Context)
	}
	if value, ok := state.mshu.Load(key); ok {
		shutdown = value.(chan struct{})
	}

	ctx = context.WithValue(ctx, keyKey, key)

	state.mstatus.Store(key, StatusRunning)

	log.Debug("[RuntimeApplication] goroutine executed", key)

	errChan := make(chan error, 1)
	defer close(errChan)

	done := make(chan struct{})
	state.wait.Add(1)
	go func() {
		defer close(done)
		log.Debug("[RuntimeApplication] goroutine start", key)
		defer func() {
			state.wait.Done()
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
			log.Debug("[RuntimeApplication] goroutine start runtime application", key)
			err = app(ctx, shutdown)
		} else {
			log.Debug("[RuntimeApplication] goroutine start retriable runtime application", key)
			err = retry.Do(func() error {
				return app(ctx, shutdown)
			}, retry.Attempts(retries))
		}
		errChan <- err
		log.Debug("[RuntimeApplication] goroutine done", key, err)
	}()

	log.Debug("[RuntimeApplication] waiting goroutine", key)

	var err error
	// if the context is cancelled => the application is cancelled
	// if the shutdown channel is closed => the application is terminated
	// if the done channel is closed => the application is stopped but maybe abrubtly
	select {
	case <-ctx.Done():
		err = ctx.Err()
	case <-shutdown:
	case <-done:
	}

	// eitherway, you check for the error
	goroutineErr := <-errChan
	if goroutineErr != nil {
		err = goroutineErr
	}

	log.Debug("[RuntimeApplication] wait done", "app", key, "err", err)

	defer func() {
		log.Debug("[RuntimeApplication] defer", key, err)
		if value, ok := state.mdone.Load(key); ok {
			close(value.(chan struct{}))
		} else {
			log.Debug("[RuntimeApplication] defer not found", key)
		}
		log.Debug("[RuntimeApplication] defer done", key)
	}()

	// Extend context cleanup
	for _, ext := range g.extensions {
		ctx = ext.OnStopRuntime(ctx)
	}

	// Check if the application is still alive
	if value, ok := state.mali.Load(key); !ok || !value.(bool) {
		return
	}

	log.Debug("[RuntimeApplication] com", key, err)

	metadata := g.getSystemMetadata()

	//	Notify to the global state
	if err != nil {
		if errors.Is(err, context.Canceled) {
			log.Debug("[RuntimeApplication] com canceled", key, err)
			_, msg := msgCancelledShutdown(key, err)
			send(metadata, msg) // sending and working on the response
		} else if errors.Is(err, ErrPanic) {
			log.Debug("[RuntimeApplication] com panic", key, err)
			_, msg := msgPanickedShutdown(key, err)
			send(metadata, msg) // final state, it is definitly finished
		} else {
			log.Debug("[RuntimeApplication] com error", key, err)
			_, msg := msgErroredShutdown(key, err) // final state, it is definitly finished
			send(metadata, msg)
		}
	} else {
		log.Debug("[RuntimeApplication] com terminate", key)
		_, msg := msgTerminatedShutdown(key)
		send(metadata, msg) // sending and working on the response
	}
}
