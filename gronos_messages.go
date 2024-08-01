package gronos

import (
	"fmt"
	"reflect"
	"runtime"
	"sync"
	"time"

	"github.com/charmbracelet/log"
)

/// Each message is a set of properties that will mutate one by one the state of the system

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

/// Functions lifecycle messages

func MsgAddRuntimeApplication[K comparable](key K, app RuntimeApplication) AddRuntimeApplicationMessage[K] {
	return AddRuntimeApplicationMessage[K]{KeyMessage[K]{key}, app}
}

func MsgCancelShutdown[K comparable](key K, err error) CancelShutdown[K] {
	return CancelShutdown[K]{KeyMessage[K]{key}, err}
}

func MsgTerminateShutdown[K comparable](key K) TerminateShutdown[K] {
	return TerminateShutdown[K]{KeyMessage[K]{key}}
}

func MsgPanicShutdown[K comparable](key K, err error) PanicShutdown[K] {
	return PanicShutdown[K]{KeyMessage[K]{key}, err}
}

func MsgErrorShutdown[K comparable](key K, err error) ErrorShutdown[K] {
	return ErrorShutdown[K]{KeyMessage[K]{key}, err}
}

func MsgAllShutdown[K comparable]() AllShutdown[K] {
	return AllShutdown[K]{}
}

func MsgAllCancelShutdown[K comparable]() AllCancelShutdown[K] {
	return AllCancelShutdown[K]{}
}

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

func MsgRequestStatus[K comparable](key K) (<-chan StatusState, RequestStatus[K]) {
	response := make(chan StatusState, 1)
	return response, RequestStatus[K]{KeyMessage[K]{key}, RequestMessage[K, StatusState]{KeyMessage[K]{key}, response}}
}

func MsgRequestAlive[K comparable](key K, response chan bool) RequestAlive[K] {
	return RequestAlive[K]{KeyMessage[K]{key}, RequestMessage[K, bool]{KeyMessage[K]{key}, response}}
}

func MsgRequestReason[K comparable](key K, response chan error) RequestReason[K] {
	return RequestReason[K]{KeyMessage[K]{key}, RequestMessage[K, error]{KeyMessage[K]{key}, response}}
}

func MsgRequestAllAlive[K comparable]() (<-chan bool, RequestAllAlive[K]) {
	response := make(chan bool, 1)
	return response, RequestAllAlive[K]{RequestMessage[K, bool]{KeyMessage[K]{}, response}}
}

func MsgRequestStatusAsync[K comparable](key K, when StatusState) (<-chan struct{}, RequestStatusAsync[K]) {
	response := make(chan struct{}, 1)
	return response, RequestStatusAsync[K]{KeyMessage[K]{key}, when, RequestMessage[K, struct{}]{KeyMessage[K]{key}, response}}
}

func (g *gronos[K]) handleGronosMessage(m Message) error {
	log.Info("[GronosMessage] handle gronos message", reflect.TypeOf(m).Name(), m)
	switch msg := m.(type) {
	case AddRuntimeApplicationMessage[K]:
		log.Info("[GronosMessage] [AddRuntimeApplicationMessage]", msg.Key)
		return g.handleAddRuntimeApplicationMessage(msg)
	case CancelShutdown[K]:
		log.Info("[GronosMessage] [CancelShutdown]", msg.Key)
		return g.handleCancelShutdown(msg)
	case AllShutdown[K]:
		log.Info("[GronosMessage] [AllShutdown]")
		return g.handleAllShutdown(msg)
	case TerminateShutdown[K]:
		log.Info("[GronosMessage] [TerminateShutdown]", msg.Key)
		return g.handleTerminateShutdown(msg)
	case RequestAllAlive[K]:
		log.Info("[GronosMessage] [RequestAllAlive]", msg.Key)
		return g.handleRequestAllAlive(msg)
	case RequestStatusAsync[K]:
		log.Info("[GronosMessage] [RequestStatusAsync]", msg.Key)
		return g.handleRequestStatusAsync(msg)
	case RequestStatus[K]:
		log.Info("[GronosMessage] [RequestStatus]", msg.Key)
		return g.handleRequestStatus(msg)
	// case terminateMessage[K]:
	// 	return g.handleDeadLetter(msg)
	// case terminatedMessage[K]:
	// 	return g.handleTerminated(msg)
	// case contextTerminatedMessage[K]:
	// 	return g.handleContextTerminated(msg)
	// case addMessage[K]:
	// 	return g.Add(msg.Key, msg.App)
	// case errorMessage[K]:
	// 	return errors.Join(fmt.Errorf("app error %v", msg.Key), msg.Err)
	// case StatusMessage[K]:
	// 	g.handleStatusMessage(msg)
	// 	return nil
	// case ShutdownMessage:
	// 	g.initiateShutdown(false)
	// 	return nil
	// case contextCancelledMessage[K]:
	// 	g.initiateShutdown(false)
	// 	return nil
	default:
		return ErrUnhandledMessage
	}
}

func (g *gronos[K]) handleRequestStatus(msg RequestStatus[K]) error {
	var value any
	var ok bool
	if value, ok = g.mstatus.Load(msg.Key); !ok {
		return fmt.Errorf("app not found (status property) %v", msg.Key)
	}
	msg.Response <- value.(StatusState)
	close(msg.Response)
	return nil
}

func (g *gronos[K]) handleRequestStatusAsync(msg RequestStatusAsync[K]) error {
	go func() {
		var currentState int
		for currentState >= stateNumber(msg.When) {
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

func (g *gronos[K]) handleRequestAllAlive(msg RequestAllAlive[K]) error {
	var alive bool
	g.mali.Range(func(key, value any) bool {
		if value.(bool) {
			alive = true
			return false
		}
		return true
	})
	msg.Response <- alive
	log.Info("[GronosMessage] all alive", alive)
	return nil
}

func (g *gronos[K]) handleTerminateShutdown(msg TerminateShutdown[K]) error {
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

	//	notifying it is really terminated
	g.mstatus.Store(msg.Key, StatusShutdownTerminated)

	g.wait.Done()

	log.Info("[GronosMessage] [TerminateShutdown] terminate shutdown terminated", msg.Key)

	return nil
}

func (g *gronos[K]) handleAllShutdown(msg AllShutdown[K]) error {

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

func (g *gronos[K]) handleAddRuntimeApplicationMessage(msg AddRuntimeApplicationMessage[K]) error {

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

// - alive to false
// - trigger `cancel` function
// - change status to StatusShutdownCancelled
func (g *gronos[K]) handleCancelShutdown(msg CancelShutdown[K]) error {
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

// // - alive to false
// // - trigger `closer` function
// // - change status to terminated
// func (g *gronos[K]) handleDeadLetter(msg terminateMessage[K]) error {
// 	var value any
// 	var ok bool
// 	if value, ok = g.mali.Load(msg.Key); !ok {
// 		return fmt.Errorf("app not found (alive property) %v", msg.Key)
// 	}
// 	//	set application to unalive
// 	if value.(bool) {
// 		g.mali.Store(msg.Key, false)
// 	}
// 	if value, ok = g.mcloser.Load(msg.Key); !ok {
// 		return fmt.Errorf("app not found (closer property) %v", msg.Key)
// 	}
// 	// trigger closer
// 	value.(func())() // the shutdown channel is now closed, runtime application is currently shutting down
// 	//	notifying it is shuting down
// 	g.mstatus.Store(msg.Key, StatusShutingDown)

// 	// value, ok := g.applications.Load(msg.Key)
// 	// if !ok {
// 	// 	return nil
// 	// }
// 	// app, ok := value.(applicationContext[K])
// 	// if !ok {
// 	// 	return nil
// 	// }
// 	// if !app.alive {
// 	// 	return nil
// 	// }
// 	// app.alive = false
// 	// app.reason = msg.Reason
// 	// app.closer()
// 	// g.applications.Store(app.k, app)
// 	// if msg.Reason != nil && msg.Reason.Error() != "shutdown" {
// 	// 	return errors.Join(fmt.Errorf("app error %v", msg.Key), msg.Reason)
// 	// }
// 	return nil
// }

// func (g *gronos[K]) initiateShutdown(cancelled bool) {
// 	if !g.isShutting.CompareAndSwap(false, true) {
// 		return // Shutdown already in progress
// 	}

// 	if g.config.shutdownBehavior == ShutdownManual && !cancelled {
// 		g.shutdownApps(false)
// 	} else {
// 		timer := time.NewTimer(g.config.gracePeriod)
// 		defer timer.Stop()

// 		shutdownChan := make(chan struct{})
// 		go g.gracefulShutdown(shutdownChan)

// 		select {
// 		case <-timer.C:
// 			g.forceShutdown()
// 		case <-shutdownChan:
// 			// Graceful shutdown completed
// 		}
// 	}
// 	g.cancel() // Cancel the context after shutdown is complete
// }
