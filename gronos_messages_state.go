package gronos

import (
	"fmt"
	"runtime"
	"sync"
	"time"

	"github.com/charmbracelet/log"
	"github.com/heimdalr/dag"
)

type MessageRequestStatus[K Primitive] struct {
	FutureMessage[KeyMessage[K], StatusState]
}

var messageRequestStatusPoolInited bool = false
var messageRequestStatusPool sync.Pool

func NewMessageRequestStatus[K Primitive](key K) *MessageRequestStatus[K] {
	if !messageRequestStatusPoolInited {
		messageRequestStatusPoolInited = true
		messageRequestStatusPool = sync.Pool{
			New: func() any {
				return &MessageRequestStatus[K]{}
			},
		}
	}
	return &MessageRequestStatus[K]{
		FutureMessage: FutureMessage[KeyMessage[K], StatusState]{
			Data:   KeyMessage[K]{Key: key},
			Result: NewFuture[StatusState](),
		},
	}
}

type MessageRequestAlive[K Primitive] struct {
	FutureMessage[KeyMessage[K], bool]
}

var messageRequestAlivePoolInited bool = false
var messageRequestAlivePool sync.Pool

func NewMessageRequestAlive[K Primitive](key K) *MessageRequestAlive[K] {
	if !messageRequestAlivePoolInited {
		messageRequestAlivePoolInited = true
		messageRequestAlivePool = sync.Pool{
			New: func() any {
				return &MessageRequestAlive[K]{}
			},
		}
	}
	return &MessageRequestAlive[K]{
		FutureMessage: FutureMessage[KeyMessage[K], bool]{
			Data:   KeyMessage[K]{Key: key},
			Result: NewFuture[bool](),
		},
	}
}

type MessageRequestReason[K Primitive] struct {
	FutureMessage[KeyMessage[K], error]
}

var messageRequestReasonPoolInited bool = false
var messageRequestReasonPool sync.Pool

func NewMessageRequestReason[K Primitive](key K) *MessageRequestReason[K] {
	if !messageRequestReasonPoolInited {
		messageRequestReasonPoolInited = true
		messageRequestReasonPool = sync.Pool{
			New: func() any {
				return &MessageRequestReason[K]{}
			},
		}
	}
	return &MessageRequestReason[K]{
		FutureMessage: FutureMessage[KeyMessage[K], error]{
			Data:   KeyMessage[K]{Key: key},
			Result: NewFuture[error](),
		},
	}
}

type MessageRequestAllAlive[K Primitive] struct {
	FutureMessage[Void, bool]
}

var messageRequestAllAlivePoolInited bool = false
var messageRequestAllAlivePool sync.Pool

func NewMessageRequestAllAlive[K Primitive]() *MessageRequestAllAlive[K] {
	if !messageRequestAllAlivePoolInited {
		messageRequestAllAlivePoolInited = true
		messageRequestAllAlivePool = sync.Pool{
			New: func() any {
				return &MessageRequestAllAlive[K]{}
			},
		}
	}
	return &MessageRequestAllAlive[K]{
		FutureMessage: FutureMessage[Void, bool]{
			Data:   Void{},
			Result: NewFuture[bool](),
		},
	}
}

type MessageRequestStatusAsync[K Primitive] struct {
	FutureMessage[struct {
		KeyMessage[K]
		When StatusState
	}, Void]
}

var messageRequestStatusAsyncPoolInited bool = false
var messageRequestStatusAsyncPool sync.Pool

func NewMessageRequestStatusAsync[K Primitive](key K, when StatusState) *MessageRequestStatusAsync[K] {
	if !messageRequestStatusAsyncPoolInited {
		messageRequestStatusAsyncPoolInited = true
		messageRequestStatusAsyncPool = sync.Pool{
			New: func() any {
				return &MessageRequestStatusAsync[K]{}
			},
		}
	}
	return &MessageRequestStatusAsync[K]{
		FutureMessage: FutureMessage[struct {
			KeyMessage[K]
			When StatusState
		}, Void]{
			Data: struct {
				KeyMessage[K]
				When StatusState
			}{
				KeyMessage: KeyMessage[K]{Key: key},
				When:       when,
			},
			Result: NewFuture[Void](),
		},
	}
}

type MessageRequestGraph[K Primitive] struct {
	FutureMessage[Void, *dag.DAG]
}

var messageRequestGraphPoolInited bool = false
var messageRequestGraphPool sync.Pool

func NewMessageRequestGraph[K Primitive]() *MessageRequestGraph[K] {
	if !messageRequestGraphPoolInited {
		messageRequestGraphPoolInited = true
		messageRequestGraphPool = sync.Pool{
			New: func() any {
				return &MessageRequestGraph[K]{}
			},
		}
	}
	return &MessageRequestGraph[K]{
		FutureMessage: FutureMessage[Void, *dag.DAG]{
			Data:   Void{},
			Result: NewFuture[*dag.DAG](),
		},
	}
}

func (g *gronos[K]) handleStateMessage(state *gronosState[K], m *MessagePayload[K]) (error, bool) {
	switch msg := m.Message.(type) {
	case *MessageRequestStatus[K]:
		log.Debug("[GronosMessage] [RequestStatus]", msg.Data.Key)
		defer messageRequestStatusPool.Put(msg)
		return g.handleRequestStatus(state, m.Metadata, msg.Data, msg.Result), true
	case *MessageRequestAlive[K]:
		log.Debug("[GronosMessage] [RequestAlive]", msg.Data.Key)
		defer messageRequestAlivePool.Put(msg)
		return g.handleRequestAlive(state, m.Metadata, msg.Data, msg.Result), true
	case *MessageRequestReason[K]:
		log.Debug("[GronosMessage] [RequestReason]", msg.Data.Key)
		defer messageRequestReasonPool.Put(msg)
		return g.handleRequestReason(state, m.Metadata, msg.Data, msg.Result), true
	case *MessageRequestAllAlive[K]:
		log.Debug("[GronosMessage] [RequestAllAlive]")
		defer messageRequestAllAlivePool.Put(msg)
		return g.handleRequestAllAlive(state, m.Metadata, msg.Data, msg.Result), true
	case *MessageRequestStatusAsync[K]: // TODO: is it still needed?
		log.Debug("[GronosMessage] [RequestStatusAsync]", msg.Data.Key)
		defer messageRequestStatusAsyncPool.Put(msg)
		return g.handleRequestStatusAsync(state, m.Metadata, msg.Data, msg.Result), true
	case *MessageRequestGraph[K]:
		log.Debug("[GronosMessage] [RequestGraph]")
		defer messageRequestGraphPool.Put(msg)
		msg.Result.Publish(state.graph)
		return nil, true
	}
	return nil, false
}

func (g *gronos[K]) handleRequestStatus(state *gronosState[K], metadata *Metadata[K], data KeyMessage[K], future Future[StatusState]) error {
	var value StatusState
	var ok bool
	if value, ok = state.mstatus.Load(data.Key); !ok {
		future.Publish(StatusNotFound)
		// return fmt.Errorf("app not found (status property) %v", key)
		return nil
	}
	future.Publish(value)
	return nil
}

func (g *gronos[K]) handleRequestAlive(state *gronosState[K], metadata *Metadata[K], data KeyMessage[K], future Future[bool]) error {
	var value bool
	var ok bool
	if value, ok = state.mali.Load(data.Key); !ok {
		return fmt.Errorf("app not found (alive property) %v", data.Key)
	}
	future.Publish(value)
	return nil
}

func (g *gronos[K]) handleRequestReason(state *gronosState[K], metadata *Metadata[K], data KeyMessage[K], future Future[error]) error {
	var value error
	var ok bool
	if value, ok = state.mrea.Load(data.Key); !ok {
		return fmt.Errorf("app not found (reason property) %v", data.Key)
	}
	future.Publish(value)
	return nil
}

func (g *gronos[K]) handleRequestAllAlive(state *gronosState[K], metadata *Metadata[K], data Void, future Future[bool]) error {
	var alive bool
	state.mali.Range(func(key K, value bool) bool {
		if value {
			alive = true
			return false
		}
		return true
	})
	future.Publish(alive)
	log.Debug("[GronosMessage] all alive", alive)
	return nil
}

func (g *gronos[K]) handleRequestStatusAsync(
	state *gronosState[K],
	metadata *Metadata[K],
	data struct {
		KeyMessage[K]
		When StatusState
	},
	future Future[Void],
) error {
	go func() {
		var currentState int
		for currentState < stateNumber(data.When) {
			if value, ok := state.mstatus.Load(data.Key); ok {
				currentState = stateNumber(value)
			}
			<-time.After(time.Second / 16)
			runtime.Gosched()
		}
		log.Debug("[GronosMessage] status async", data.Key, currentState)
		future.Close()
	}()
	return nil
}
