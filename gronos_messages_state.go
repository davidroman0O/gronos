package gronos

import (
	"fmt"
	"runtime"
	"sync"
	"time"

	"github.com/charmbracelet/log"
)

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

var requestStatusPoolInited bool
var requestStatusPool sync.Pool

var requestAlivePoolInited bool
var requestAlivePool sync.Pool

var requestReasonPoolInited bool
var requestReasonPool sync.Pool

var requestAllAlivePoolInited bool
var requestAllAlivePool sync.Pool

var requestStatusAsyncPoolInited bool
var requestStatusAsyncPool sync.Pool

func MsgRequestStatus[K comparable](key K) (<-chan StatusState, *RequestStatus[K]) {
	if !requestStatusPoolInited {
		requestStatusPoolInited = true
		requestStatusPool = sync.Pool{
			New: func() any {
				return &RequestStatus[K]{}
			},
		}
	}
	response := make(chan StatusState, 1)
	msg := requestStatusPool.Get().(*RequestStatus[K])
	msg.Key = key
	msg.Response = response
	return response, msg
}

func MsgRequestAlive[K comparable](key K) *RequestAlive[K] {
	if !requestAlivePoolInited {
		requestAlivePoolInited = true
		requestAlivePool = sync.Pool{
			New: func() any {
				return &RequestAlive[K]{}
			},
		}
	}
	msg := requestAlivePool.Get().(*RequestAlive[K])
	msg.Key = key
	msg.Response = make(chan bool, 1)
	return msg
}

func MsgRequestReason[K comparable](key K) *RequestReason[K] {
	if !requestReasonPoolInited {
		requestReasonPoolInited = true
		requestReasonPool = sync.Pool{
			New: func() any {
				return &RequestReason[K]{}
			},
		}
	}
	msg := requestReasonPool.Get().(*RequestReason[K])
	msg.Key = key
	msg.Response = make(chan error, 1)
	return msg
}

func MsgRequestAllAlive[K comparable]() (<-chan bool, *RequestAllAlive[K]) {
	if !requestAllAlivePoolInited {
		requestAllAlivePoolInited = true
		requestAllAlivePool = sync.Pool{
			New: func() any {
				return &RequestAllAlive[K]{}
			},
		}
	}
	response := make(chan bool, 1)
	msg := requestAllAlivePool.Get().(*RequestAllAlive[K])
	msg.Response = response
	return response, msg
}

func MsgRequestStatusAsync[K comparable](key K, when StatusState) (<-chan struct{}, *RequestStatusAsync[K]) {
	if !requestStatusAsyncPoolInited {
		requestStatusAsyncPoolInited = true
		requestStatusAsyncPool = sync.Pool{
			New: func() any {
				return &RequestStatusAsync[K]{}
			},
		}
	}
	response := make(chan struct{}, 1)
	msg := requestStatusAsyncPool.Get().(*RequestStatusAsync[K])
	msg.Key = key
	msg.When = when
	msg.Response = response
	return response, msg
}

func (g *gronos[K]) handleStateMessage(state *gronosState[K], m *MessagePayload) (error, bool) {
	switch msg := m.Message.(type) {
	case *RequestStatus[K]:
		log.Debug("[GronosMessage] [RequestStatus]", msg.Key)
		defer requestStatusPool.Put(msg)
		return g.handleRequestStatus(state, msg.Key, msg.Response), true
	case *RequestAlive[K]:
		log.Debug("[GronosMessage] [RequestAlive]", msg.Key)
		defer requestAlivePool.Put(msg)
		return g.handleRequestAlive(state, msg.Key, msg.Response), true
	case *RequestReason[K]:
		log.Debug("[GronosMessage] [RequestReason]", msg.Key)
		defer requestReasonPool.Put(msg)
		return g.handleRequestReason(state, msg.Key, msg.Response), true
	case *RequestAllAlive[K]:
		log.Debug("[GronosMessage] [RequestAllAlive]")
		defer requestAllAlivePool.Put(msg)
		return g.handleRequestAllAlive(state, msg.Response), true
	case *RequestStatusAsync[K]:
		log.Debug("[GronosMessage] [RequestStatusAsync]", msg.Key)
		defer requestStatusAsyncPool.Put(msg)
		return g.handleRequestStatusAsync(state, msg.Key, msg.When, msg.Response), true
	}
	return nil, false
}

func (g *gronos[K]) handleRequestStatus(state *gronosState[K], key K, response chan<- StatusState) error {
	defer close(response)
	var value any
	var ok bool
	if value, ok = state.mstatus.Load(key); !ok {
		response <- StatusNotFound
		// return fmt.Errorf("app not found (status property) %v", key)
		return nil
	}
	response <- value.(StatusState)
	return nil
}

func (g *gronos[K]) handleRequestAlive(state *gronosState[K], key K, response chan<- bool) error {
	var value any
	var ok bool
	if value, ok = state.mali.Load(key); !ok {
		return fmt.Errorf("app not found (alive property) %v", key)
	}
	response <- value.(bool)
	close(response)
	return nil
}

func (g *gronos[K]) handleRequestReason(state *gronosState[K], key K, response chan<- error) error {
	var value any
	var ok bool
	if value, ok = state.mrea.Load(key); !ok {
		return fmt.Errorf("app not found (reason property) %v", key)
	}
	response <- value.(error)
	close(response)
	return nil
}

func (g *gronos[K]) handleRequestAllAlive(state *gronosState[K], response chan<- bool) error {
	var alive bool
	state.mali.Range(func(key, value any) bool {
		if value.(bool) {
			alive = true
			return false
		}
		return true
	})
	response <- alive
	close(response)
	log.Debug("[GronosMessage] all alive", alive)
	return nil
}

func (g *gronos[K]) handleRequestStatusAsync(state *gronosState[K], key K, when StatusState, response chan<- struct{}) error {
	go func() {
		var currentState int
		for currentState < stateNumber(when) {
			if value, ok := state.mstatus.Load(key); ok {
				currentState = stateNumber(value.(StatusState))
			}
			<-time.After(time.Second / 16)
			runtime.Gosched()
		}
		log.Debug("[GronosMessage] status async", key, currentState)
		close(response)
	}()
	return nil
}
