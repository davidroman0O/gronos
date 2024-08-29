package gronos

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/avast/retry-go/v3"
	"github.com/charmbracelet/log"
)

type MessageProcessor[K Primitive, D any, F any] func(state *gronosState[K], metadata *Metadata[K], data D, future Future[F]) (error, bool)

type MessagePoolProcessor[K Primitive, D any, F any] struct {
	pool      sync.Pool
	processor MessageProcessor[K, D, F]
}

type MessageAddLifecycleFunction[K Primitive] struct {
	FutureMessage[
		struct {
			KeyMessage[K]
			LifecyleFunc
		},
		Void,
	]
}

var messageAddLifecycleFunctionPoolInited bool = false
var messageAddLifecycleFunctionPool sync.Pool

func NewMessageAddLifecycleFunction[K Primitive](
	key K,
	app LifecyleFunc,
) *MessageAddLifecycleFunction[K] {
	if !messageAddLifecycleFunctionPoolInited {
		messageAddLifecycleFunctionPoolInited = true
		messageAddLifecycleFunctionPool = sync.Pool{
			New: func() any {
				return &MessageAddLifecycleFunction[K]{}
			},
		}
	}
	return &MessageAddLifecycleFunction[K]{
		FutureMessage: FutureMessage[struct {
			KeyMessage[K]
			LifecyleFunc
		}, Void]{
			Data: struct {
				KeyMessage[K]
				LifecyleFunc
			}{
				KeyMessage:   KeyMessage[K]{Key: key},
				LifecyleFunc: app,
			},
			Result: NewFuture[Void](),
		},
	}
}

// type MessageProcessorAddLifecycleFunction[K Primitive] struct {
// 	MessagePoolProcessor[K,
// 		struct {
// 			KeyMessage[K]
// 			LifecyleFunc
// 		},
// 		Void]
// }

// type RemoveMessage[K Primitive] struct {
// 	KeyMessage[K]
// 	RequestMessage[K, bool]
// }

// type RuntimeError[K Primitive] struct {
// 	KeyMessage[K]
// 	Error error
// }

// func MsgRuntimeError[K Primitive](key K, err error) *RuntimeError[K] {
// 	return &RuntimeError[K]{KeyMessage[K]{key}, err}
// }

// // global system force cancel
// type ForceCancelShutdown[K Primitive] struct {
// 	KeyMessage[K]
// 	Error error
// 	RequestMessage[K, struct{}]
// }

// // global system force terminate
// type ForceTerminateShutdown[K Primitive] struct {
// 	KeyMessage[K]
// 	RequestMessage[K, struct{}]
// }

// type CancelledShutdown[K Primitive] struct {
// 	KeyMessage[K]
// 	Error error
// 	RequestMessage[K, struct{}]
// }

// type TerminatedShutdown[K Primitive] struct {
// 	RequestMessage[K, struct{}]
// }

// type PanickedShutdown[K Primitive] struct {
// 	KeyMessage[K]
// 	Error error
// 	RequestMessage[K, struct{}]
// }

// type ErroredShutdown[K Primitive] struct {
// 	KeyMessage[K]
// 	Error error
// 	RequestMessage[K, struct{}]
// }

// type GetListRuntimeApplication[K Primitive] struct {
// 	RequestMessage[K, []K]
// }

// var addRuntimeApplicationPoolInited bool = false
// var addRuntimeApplicationPool sync.Pool

// var removeRuntimeApplicationPoolInited bool = false
// var removeRuntimeApplicationPool sync.Pool

// var cancelShutdownPoolInited bool = false
// var cancelShutdownPool sync.Pool

// var terminatedShutdownPoolInited bool = false
// var terminatedShutdownPool sync.Pool

// var forceCancelShutdownPoolInited bool = false
// var forceCancelShutdownPool sync.Pool

// var forceTerminateShutdownPoolInited bool = false
// var forceTerminateShutdownPool sync.Pool

// var panicShutdownPoolInited bool = false
// var panickedShutdownPool sync.Pool

// var erroredShutdownPoolInited bool = false
// var erroredShutdownPool sync.Pool

// var getListRuntimeApplicationPoolInited bool = false
// var getListRuntimeApplicationPool sync.Pool

// func MsgGetListRuntimeApplication[K Primitive]() (<-chan []K, *GetListRuntimeApplication[K]) {
// 	if !getListRuntimeApplicationPoolInited {
// 		getListRuntimeApplicationPoolInited = true
// 		getListRuntimeApplicationPool = sync.Pool{
// 			New: func() any {
// 				return &GetListRuntimeApplication[K]{}
// 			},
// 		}
// 	}
// 	msg := getListRuntimeApplicationPool.Get().(*GetListRuntimeApplication[K])
// 	msg.Response = make(chan []K, 1)
// 	return msg.Response, msg
// }

// // func MsgAdd[K Primitive](key K, app LifecyleFunc) (<-chan struct{}, *AddMessage[K]) {
// // 	if !addRuntimeApplicationPoolInited {
// // 		addRuntimeApplicationPoolInited = true
// // 		addRuntimeApplicationPool = sync.Pool{
// // 			New: func() any {
// // 				return &AddMessage[K]{}
// // 			},
// // 		}
// // 	}
// // 	msg := addRuntimeApplicationPool.Get().(*AddMessage[K])
// // 	msg.Key = key
// // 	msg.LifecyleFunc = app
// // 	msg.Response = make(chan struct{}, 1)
// // 	return msg.Response, msg
// // }

// func MsgRemove[K Primitive](key K) (<-chan bool, *RemoveMessage[K]) {
// 	if !removeRuntimeApplicationPoolInited {
// 		removeRuntimeApplicationPoolInited = true
// 		removeRuntimeApplicationPool = sync.Pool{
// 			New: func() any {
// 				return &RemoveMessage[K]{}
// 			},
// 		}
// 	}
// 	msg := removeRuntimeApplicationPool.Get().(*RemoveMessage[K])
// 	msg.Key = key
// 	msg.Response = make(chan bool, 1)
// 	return msg.Response, msg
// }

// func MsgForceCancelShutdown[K Primitive](key K, err error) (<-chan struct{}, *ForceCancelShutdown[K]) {
// 	if !forceCancelShutdownPoolInited {
// 		forceCancelShutdownPoolInited = true
// 		forceCancelShutdownPool = sync.Pool{
// 			New: func() any {
// 				return &ForceCancelShutdown[K]{}
// 			},
// 		}
// 	}
// 	msg := forceCancelShutdownPool.Get().(*ForceCancelShutdown[K])
// 	msg.Key = key
// 	msg.Error = err
// 	msg.Response = make(chan struct{}, 1)
// 	return msg.Response, msg
// }

// func MsgForceTerminateShutdown[K Primitive](key K) (<-chan struct{}, *ForceTerminateShutdown[K]) {
// 	if !forceTerminateShutdownPoolInited {
// 		forceTerminateShutdownPoolInited = true
// 		forceTerminateShutdownPool = sync.Pool{
// 			New: func() any {
// 				return &ForceTerminateShutdown[K]{}
// 			},
// 		}
// 	}
// 	msg := forceTerminateShutdownPool.Get().(*ForceTerminateShutdown[K])
// 	msg.Key = key
// 	msg.Response = make(chan struct{}, 1)
// 	return msg.Response, msg
// }

// func msgCancelledShutdown[K Primitive](key K, err error) (<-chan struct{}, *CancelledShutdown[K]) {
// 	if !cancelShutdownPoolInited {
// 		cancelShutdownPoolInited = true
// 		cancelShutdownPool = sync.Pool{
// 			New: func() any {
// 				return &CancelledShutdown[K]{}
// 			},
// 		}
// 	}
// 	msg := cancelShutdownPool.Get().(*CancelledShutdown[K])
// 	msg.Key = key
// 	msg.Error = err
// 	response := make(chan struct{}, 1)
// 	msg.Response = response
// 	return response, msg
// }

// func msgTerminatedShutdown[K Primitive](key K) (<-chan struct{}, *TerminatedShutdown[K]) {
// 	if !terminatedShutdownPoolInited {
// 		terminatedShutdownPoolInited = true
// 		terminatedShutdownPool = sync.Pool{
// 			New: func() any {
// 				return &TerminatedShutdown[K]{}
// 			},
// 		}
// 	}
// 	msg := terminatedShutdownPool.Get().(*TerminatedShutdown[K])
// 	msg.Key = key
// 	response := make(chan struct{}, 1)
// 	msg.Response = response
// 	return response, msg
// }

// func msgPanickedShutdown[K Primitive](key K, err error) (<-chan struct{}, *PanickedShutdown[K]) {
// 	if !panicShutdownPoolInited {
// 		panicShutdownPoolInited = true
// 		panickedShutdownPool = sync.Pool{
// 			New: func() any {
// 				return &PanickedShutdown[K]{}
// 			},
// 		}
// 	}
// 	msg := panickedShutdownPool.Get().(*PanickedShutdown[K])
// 	msg.Key = key
// 	msg.Error = err
// 	response := make(chan struct{}, 1)
// 	msg.Response = response
// 	return response, msg
// }

// func msgErroredShutdown[K Primitive](key K, err error) (<-chan struct{}, *ErroredShutdown[K]) {
// 	if !erroredShutdownPoolInited {
// 		erroredShutdownPoolInited = true
// 		erroredShutdownPool = sync.Pool{
// 			New: func() any {
// 				return &ErroredShutdown[K]{}
// 			},
// 		}
// 	}
// 	msg := erroredShutdownPool.Get().(*ErroredShutdown[K])
// 	msg.Key = key
// 	msg.Error = err
// 	response := make(chan struct{}, 1)
// 	msg.Response = response
// 	return response, msg
// }

type MessageRemoveLifecycleFunction[K Primitive] struct {
	FutureMessage[KeyMessage[K], bool]
}

var messageRemoveLifecycleFunctionPoolInited bool = false
var messageRemoveLifecycleFunctionPool sync.Pool

func NewMessageRemoveLifecycleFunction[K Primitive](key K) *MessageRemoveLifecycleFunction[K] {
	if !messageRemoveLifecycleFunctionPoolInited {
		messageRemoveLifecycleFunctionPoolInited = true
		messageRemoveLifecycleFunctionPool = sync.Pool{
			New: func() any {
				return &MessageRemoveLifecycleFunction[K]{}
			},
		}
	}
	return &MessageRemoveLifecycleFunction[K]{
		FutureMessage: FutureMessage[KeyMessage[K], bool]{
			Data:   KeyMessage[K]{Key: key},
			Result: NewFuture[bool](),
		},
	}
}

type MessageRuntimeError[K Primitive] struct {
	FutureMessage[struct {
		KeyMessage[K]
		Error error
	}, Void]
}

var messageRuntimeErrorPoolInited bool = false
var messageRuntimeErrorPool sync.Pool

func NewMessageRuntimeError[K Primitive](key K, err error) *MessageRuntimeError[K] {
	if !messageRuntimeErrorPoolInited {
		messageRuntimeErrorPoolInited = true
		messageRuntimeErrorPool = sync.Pool{
			New: func() any {
				return &MessageRuntimeError[K]{}
			},
		}
	}
	return &MessageRuntimeError[K]{
		FutureMessage: FutureMessage[struct {
			KeyMessage[K]
			Error error
		}, Void]{
			Data: struct {
				KeyMessage[K]
				Error error
			}{
				KeyMessage: KeyMessage[K]{Key: key},
				Error:      err,
			},
			Result: NewFuture[Void](),
		},
	}
}

type MessageForceCancelShutdown[K Primitive] struct {
	FutureMessage[struct {
		KeyMessage[K]
		Error error
	}, Void]
}

var messageForceCancelShutdownPoolInited bool = false
var messageForceCancelShutdownPool sync.Pool

func NewMessageForceCancelShutdown[K Primitive](key K, err error) *MessageForceCancelShutdown[K] {
	if !messageForceCancelShutdownPoolInited {
		messageForceCancelShutdownPoolInited = true
		messageForceCancelShutdownPool = sync.Pool{
			New: func() any {
				return &MessageForceCancelShutdown[K]{}
			},
		}
	}
	return &MessageForceCancelShutdown[K]{
		FutureMessage: FutureMessage[struct {
			KeyMessage[K]
			Error error
		}, Void]{
			Data: struct {
				KeyMessage[K]
				Error error
			}{
				KeyMessage: KeyMessage[K]{Key: key},
				Error:      err,
			},
			Result: NewFuture[Void](),
		},
	}
}

type MessageForceTerminateShutdown[K Primitive] struct {
	FutureMessage[KeyMessage[K], Void]
}

var messageForceTerminateShutdownPoolInited bool = false
var messageForceTerminateShutdownPool sync.Pool

func NewMessageForceTerminateShutdown[K Primitive](key K) *MessageForceTerminateShutdown[K] {
	if !messageForceTerminateShutdownPoolInited {
		messageForceTerminateShutdownPoolInited = true
		messageForceTerminateShutdownPool = sync.Pool{
			New: func() any {
				return &MessageForceTerminateShutdown[K]{}
			},
		}
	}
	return &MessageForceTerminateShutdown[K]{
		FutureMessage: FutureMessage[KeyMessage[K], Void]{
			Data:   KeyMessage[K]{Key: key},
			Result: NewFuture[Void](),
		},
	}
}

type MessageCancelledShutdown[K Primitive] struct {
	FutureMessage[struct {
		KeyMessage[K]
		Error error
	}, Void]
}

var messageCancelledShutdownPoolInited bool = false
var messageCancelledShutdownPool sync.Pool

func NewMessageCancelledShutdown[K Primitive](key K, err error) *MessageCancelledShutdown[K] {
	if !messageCancelledShutdownPoolInited {
		messageCancelledShutdownPoolInited = true
		messageCancelledShutdownPool = sync.Pool{
			New: func() any {
				return &MessageCancelledShutdown[K]{}
			},
		}
	}
	return &MessageCancelledShutdown[K]{
		FutureMessage: FutureMessage[struct {
			KeyMessage[K]
			Error error
		}, Void]{
			Data: struct {
				KeyMessage[K]
				Error error
			}{
				KeyMessage: KeyMessage[K]{Key: key},
				Error:      err,
			},
			Result: NewFuture[Void](),
		},
	}
}

type MessageTerminatedShutdown[K Primitive] struct {
	FutureMessage[KeyMessage[K], Void]
}

var messageTerminatedShutdownPoolInited bool = false
var messageTerminatedShutdownPool sync.Pool

func NewMessageTerminatedShutdown[K Primitive](key K) *MessageTerminatedShutdown[K] {
	if !messageTerminatedShutdownPoolInited {
		messageTerminatedShutdownPoolInited = true
		messageTerminatedShutdownPool = sync.Pool{
			New: func() any {
				return &MessageTerminatedShutdown[K]{}
			},
		}
	}
	return &MessageTerminatedShutdown[K]{
		FutureMessage: FutureMessage[KeyMessage[K], Void]{
			Data:   KeyMessage[K]{Key: key},
			Result: NewFuture[Void](),
		},
	}
}

type MessagePanickedShutdown[K Primitive] struct {
	FutureMessage[struct {
		KeyMessage[K]
		Error error
	}, Void]
}

var messagePanickedShutdownPoolInited bool = false
var messagePanickedShutdownPool sync.Pool

func NewMessagePanickedShutdown[K Primitive](key K, err error) *MessagePanickedShutdown[K] {
	if !messagePanickedShutdownPoolInited {
		messagePanickedShutdownPoolInited = true
		messagePanickedShutdownPool = sync.Pool{
			New: func() any {
				return &MessagePanickedShutdown[K]{}
			},
		}
	}
	return &MessagePanickedShutdown[K]{
		FutureMessage: FutureMessage[struct {
			KeyMessage[K]
			Error error
		}, Void]{
			Data: struct {
				KeyMessage[K]
				Error error
			}{
				KeyMessage: KeyMessage[K]{Key: key},
				Error:      err,
			},
			Result: NewFuture[Void](),
		},
	}
}

type MessageErroredShutdown[K Primitive] struct {
	FutureMessage[struct {
		KeyMessage[K]
		Error error
	}, Void]
}

var messageErroredShutdownPoolInited bool = false
var messageErroredShutdownPool sync.Pool

func NewMessageErroredShutdown[K Primitive](key K, err error) *MessageErroredShutdown[K] {
	if !messageErroredShutdownPoolInited {
		messageErroredShutdownPoolInited = true
		messageErroredShutdownPool = sync.Pool{
			New: func() any {
				return &MessageErroredShutdown[K]{}
			},
		}
	}
	return &MessageErroredShutdown[K]{
		FutureMessage: FutureMessage[struct {
			KeyMessage[K]
			Error error
		}, Void]{
			Data: struct {
				KeyMessage[K]
				Error error
			}{
				KeyMessage: KeyMessage[K]{Key: key},
				Error:      err,
			},
			Result: NewFuture[Void](),
		},
	}
}

type MessageGetListRuntimeApplication[K Primitive] struct {
	FutureMessage[Void, []K]
}

var messageGetListRuntimeApplicationPoolInited bool = false
var messageGetListRuntimeApplicationPool sync.Pool

func NewMessageGetListRuntimeApplication[K Primitive]() *MessageGetListRuntimeApplication[K] {
	if !messageGetListRuntimeApplicationPoolInited {
		messageGetListRuntimeApplicationPoolInited = true
		messageGetListRuntimeApplicationPool = sync.Pool{
			New: func() any {
				return &MessageGetListRuntimeApplication[K]{}
			},
		}
	}
	return &MessageGetListRuntimeApplication[K]{
		FutureMessage: FutureMessage[Void, []K]{
			Data:   Void{},
			Result: NewFuture[[]K](),
		},
	}
}

func (g *gronos[K]) handleRuntimeApplicationMessage(state *gronosState[K], m *MessagePayload[K]) (error, bool) {
	switch msg := m.Message.(type) {
	case *MessageAddLifecycleFunction[K]:
		log.Debug("[GronosMessage] [AddMessage]", "key", msg.Data.Key, "metadata", m.Metadata.String())
		defer messageAddLifecycleFunctionPool.Put(msg)
		return g.handleAddRuntimeApplication(state, m.Metadata, msg.Data, msg.Result), true
	case *MessageRemoveLifecycleFunction[K]:
		log.Debug("[GronosMessage] [RemoveMessage]", msg.Data.Key)
		defer messageRemoveLifecycleFunctionPool.Put(msg)
		return g.handleRemoveRuntimeApplication(state, m.Metadata, msg.Data, msg.Result), true
	case *MessageCancelledShutdown[K]:
		log.Debug("[GronosMessage] [CancelShutdown]", msg.Data.Key)
		defer messageCancelledShutdownPool.Put(msg)
		return g.handleCancelledShutdown(state, m.Metadata, msg.Data, msg.Result), true
	case *MessageTerminatedShutdown[K]:
		log.Debug("[GronosMessage] [TerminateShutdown]", msg.Data.Key)
		defer messageTerminatedShutdownPool.Put(msg)
		return g.handleTerminateShutdown(state, m.Metadata, msg.Data, msg.Result), true
	case *MessagePanickedShutdown[K]:
		log.Debug("[GronosMessage] [PanicShutdown]")
		defer messagePanickedShutdownPool.Put(msg)
		return g.handlePanicShutdown(state, m.Metadata, msg.Data, msg.Result), true
	case *MessageErroredShutdown[K]:
		log.Debug("[GronosMessage] [ErrorShutdown]")
		defer messageErroredShutdownPool.Put(msg)
		return g.handleErrorShutdown(state, m.Metadata, msg.Data, msg.Result), true
	case *MessageForceCancelShutdown[K]:
		log.Debug("[GronosMessage] [ForceCancelShutdown]")
		defer messageForceCancelShutdownPool.Put(msg)
		return g.handleForceCancelShutdown(state, m.Metadata, msg.Data, msg.Result), true
	case *MessageForceTerminateShutdown[K]:
		defer messageForceTerminateShutdownPool.Put(msg)
		log.Debug("[GronosMessage] [ForceTerminateShutdown]")
		return g.handleForceTerminateShutdown(state, m.Metadata, msg.Data, msg.Result), true
	case *MessageGetListRuntimeApplication[K]:
		log.Debug("[GronosMessage] [GetListRuntimeApplication]")
		defer messageGetListRuntimeApplicationPool.Put(msg)
		return g.handleRequestListRuntimeApplication(state, m.Metadata, msg.Data, msg.Result), true
	default:
		return nil, false
	}
}

func (g *gronos[K]) handleRequestListRuntimeApplication(state *gronosState[K], metadata *Metadata[K], data Void, future Future[[]K]) error {
	var list []K
	state.mkeys.Range(func(k, v K) bool {
		list = append(list, k)
		return true
	})
	future.Publish(list)
	return nil
}

// need to check if the have it or not, terminate it and remove it
// it's an async process
func (g *gronos[K]) handleRemoveRuntimeApplication(state *gronosState[K], metadata *Metadata[K], data KeyMessage[K], future Future[bool]) error {

	if state.shutting.Load() {
		return fmt.Errorf("gronos is shutting down")
	}

	remove := func() error {
		state.mkeys.Delete(data.Key)
		state.mapp.Delete(data.Key)
		state.mctx.Delete(data.Key)
		state.mcom.Delete(data.Key)
		state.mshu.Delete(data.Key)
		state.mali.Delete(data.Key)
		state.mrea.Delete(data.Key)
		state.mstatus.Delete(data.Key)
		state.mret.Delete(data.Key)
		state.mdone.Delete(data.Key)
		state.mcloser.Delete(data.Key)
		state.mcancel.Delete(data.Key)
		if err := state.graph.DeleteVertex(fmt.Sprintf("%v", data.Key)); err != nil {
			log.Error("[GronosMessage] [RemoveMessage] failed to delete vertex", data.Key, err)
			return err
		}
		log.Debug("[GronosMessage] [RemoveMessage] application removed", data.Key)
		future.Publish(true)
		return nil
	}

	log.Debug("[GronosMessage] [RemoveMessage] remove application", data.Key)

	var alive bool
	var ok bool
	if alive, ok = state.mali.Load(data.Key); !ok {
		log.Debug("[GronosMessage] [RemoveMessage] malive application not found", data.Key) // but that's fine
		return remove()
	}
	if !alive {
		log.Debug("[GronosMessage] [RemoveMessage] application already dead", data.Key) // but that's fine
		return remove()
	} else {
		log.Debug("[GronosMessage] [RemoveMessage] try to remove when terminated asynchronously", data.Key)

		msg := g.enqueue(ChannelTypePublic, metadata, NewMessageForceTerminateShutdown(data.Key))
		go func() {
			switch value := msg.WaitWithTimeout(time.Second).(type) {
			case Failure:
				log.Debug("[GronosMessage] [RemoveMessage] failed to terminate application", data.Key, value.Err)
				g.enqueue(ChannelTypePublic, metadata, NewMessageRuntimeError(data.Key, value.Err))
				return
			}
			if err := remove(); err != nil {
				log.Debug("[GronosMessage] [RemoveMessage] failed to remove application", data.Key, err)
				g.enqueue(ChannelTypePublic, metadata, NewMessageRuntimeError(data.Key, err))
			}
		}()

		// go func() {

		// 	// then it will be async
		// 	go func(whenTerminated <-chan struct{}) {
		// 		log.Debug("[GronosMessage] [RemoveMessage] terminate application", key)
		// 		<-whenTerminated
		// 		if err := remove(); err != nil {
		// 			// TODO: send to error channel
		// 			log.Debug("[GronosMessage] [RemoveMessage] failed to remove application", key, err)
		// 		}
		// 	}(
		// 		g.sendMessageWait(metadata, func() (<-chan struct{}, Message) {
		// 			return MsgForceTerminateShutdown(key)
		// 		}),
		// 	)
		// }()
		return nil
	}
}

func (g *gronos[K]) handleAddRuntimeApplication(
	state *gronosState[K],
	metadata *Metadata[K],
	data struct {
		KeyMessage[K]
		LifecyleFunc
	},
	future Future[Void],
) error {

	if state.shutting.Load() {
		return fmt.Errorf("gronos is shutting down")
	}

	log.Debug("[GronosMessage] [AddMessage] add application", data.Key)

	if _, ok := state.mkeys.Load(data.Key); ok {
		future.PublishError(fmt.Errorf("application with key %v already exists", data.Key))
		return fmt.Errorf("application with key %v already exists", data.Key)
	}
	if g.ctx.Err() != nil {
		return g.ctx.Err()
	}
	if g.isShutting.Load() {
		future.PublishError(fmt.Errorf("gronos is shutting down"))
		return fmt.Errorf("gronos is shutting down")
	}

	ctx, cancel := g.createContext(data.Key)
	shutdown := make(chan struct{})

	log.Debug("[GronosMessage] [AddMessage] add application with extensions", data.Key, data.LifecyleFunc)
	for _, ext := range g.extensions {
		ctx = ext.OnNewRuntime(ctx)
	}

	metadata.data.Range(func(k, v any) bool {
		log.Debug("[GronosMessage] [AddMessage] add application with metadata", "key", data.Key, k, v)
		ctx = context.WithValue(ctx, k, v)
		return true
	})

	state.mkeys.Store(data.Key, data.Key)
	state.mapp.Store(data.Key, data.LifecyleFunc)
	state.mctx.Store(data.Key, ctx)
	state.mcom.Store(data.Key, g.publicChn)
	state.mshu.Store(data.Key, shutdown)
	state.mali.Store(data.Key, true)
	state.mrea.Store(data.Key, nil)
	state.mstatus.Store(data.Key, StatusAdded)

	var retries uint = 0
	state.mret.Store(data.Key, retries)

	realDone := make(chan struct{})
	state.mdone.Store(data.Key, realDone)

	state.mcloser.Store(data.Key, sync.OnceFunc(func() {
		log.Debug("[GronosMessage] [AddMessage] close", data.Key)
		close(shutdown)
	}))

	state.mcancel.Store(data.Key, sync.OnceFunc(func() {
		log.Debug("[GronosMessage] [AddMessage] cancel", data.Key)
		cancel()
	}))

	go g.handleRuntimeApplication(state, metadata, data.Key)

	log.Debug("[GronosMessage] [AddMessage] application added", data.Key)

	future.Close()

	return nil
}

func (g *gronos[K]) handleForceCancelShutdown(
	state *gronosState[K],
	metadata *Metadata[K],
	data struct {
		KeyMessage[K]
		Error error
	},
	future Future[Void],
) error {

	var alive bool
	var ok bool

	if alive, ok = state.mali.Load(data.Key); !ok {
		log.Debug("[GronosMessage] [ForceCancelShutdown] app not found (alive property)", data.Key)
		future.PublishError(fmt.Errorf("app not found (alive property) %v", data.Key))
		return fmt.Errorf("app not found (alive property) %v", data.Key)
	}

	if !alive {
		log.Debug("[GronosMessage] [ForceCancelShutdown] app already dead", data.Key)
		future.PublishError(fmt.Errorf("app already dead %v", data.Key))
		return fmt.Errorf("app already dead %v", data.Key)
	}

	var cancel func()
	log.Debug("[GronosMessage] [ForceCancelShutdown] cancel", data.Key, data.Error)

	if cancel, ok = state.mcancel.Load(data.Key); !ok {
		log.Debug("[GronosMessage] [ForceCancelShutdown] app not found (cancel property)", data.Key)
		future.PublishError(fmt.Errorf("app not found (cancel property) %v", data.Key))
		return fmt.Errorf("app not found (closer property) %v", data.Key)
	}

	state.mstatus.Store(data.Key, StatusShutingDown)

	cancel()

	state.mstatus.Store(data.Key, StatusShutingDown)
	log.Debug("[GronosMessage] [ForceCancelShutdown] cancel done", data.Key)

	future.Close()

	return nil
}

func (g *gronos[K]) handleForceTerminateShutdown(state *gronosState[K], metadata *Metadata[K], data KeyMessage[K], future Future[Void]) error {

	var alive bool
	var ok bool
	if alive, ok = state.mali.Load(data.Key); !ok {
		future.PublishError(fmt.Errorf("app not found (alive property) %v", data.Key))
		return fmt.Errorf("app not found (alive property) %v", data.Key)
	}
	if !alive {
		future.PublishError(fmt.Errorf("app already dead %v", data.Key))
		return fmt.Errorf("app already dead %v", data.Key)
	}

	var closer func()
	log.Debug("[GronosMessage] [ForceTerminateShutdown] terminate shutdown", data.Key)
	if closer, ok = state.mcloser.Load(data.Key); !ok {
		future.PublishError(fmt.Errorf("app not found (closer property) %v", data.Key))
		return fmt.Errorf("app not found (closer property) %v", data.Key)
	}

	closer()

	state.mstatus.Store(data.Key, StatusShutingDown)

	log.Debug("[GronosMessage] [ForceTerminateShutdown] terminate shutdown done", data.Key)

	future.Close()

	return nil
}

func (g *gronos[K]) handleCancelledShutdown(
	state *gronosState[K],
	metadata *Metadata[K],
	data struct {
		KeyMessage[K]
		Error error
	},
	future Future[Void],
) error {

	var alive bool
	var ok bool

	if alive, ok = state.mali.Load(data.Key); !ok {
		return fmt.Errorf("app not found (alive property) %v", data.Key)
	}
	if !alive {
		return fmt.Errorf("app already dead %v", data.Key)
	}

	var chnDone chan struct{}
	if chnDone, ok = state.mdone.Load(data.Key); !ok {
		return fmt.Errorf("app not found (done property) %v", data.Key)
	}

	log.Debug("[GronosMessage] [CancelledShutdown] terminate cancelled shutdown waiting real done", data.Key)

	go func() {

		var metadata *Metadata[K]
		switch value := g.getSystemMetadata().(type) {
		case Success[*Metadata[K]]:
			metadata = value.Value
		case Failure:
			future.PublishError(value.Err)
			return
		}

		<-chnDone
		log.Debug("[GronosMessage] [CancelledShutdown] terminate cancelled shutdown done", data.Key, ok)

		if alive, ok = state.mali.Load(data.Key); !ok {
			log.Debug("[GronosMessage] [CancelledShutdown] app not found (alive property)", data.Key)
			g.enqueue(ChannelTypePublic, metadata, NewMessageRuntimeError(data.Key, fmt.Errorf("app not found (alive property) %v", data.Key)))
			// g.sendMessage(metadata, MsgRuntimeError(data.Key, fmt.Errorf("app not found (alive property) %v", data.Key)))
			future.PublishError(fmt.Errorf("app not found (alive property) %v", data.Key))
			return
		}

		if alive {
			state.mali.Store(data.Key, false)
		}

		state.mstatus.Store(data.Key, StatusShutdownCancelled)

		future.Close()

		log.Debug("[GronosMessage] [CancelledShutdown] terminate cancelled shutdown terminated", data.Key)

	}()

	return nil
}

func (g *gronos[K]) handleTerminateShutdown(state *gronosState[K], metadata *Metadata[K], data KeyMessage[K], future Future[Void]) error {
	var alive bool
	var ok bool
	if alive, ok = state.mali.Load(data.Key); !ok {
		return fmt.Errorf("app not found (alive property) %v", data.Key)
	}
	if !alive {
		return fmt.Errorf("app already dead %v", data.Key)
	}

	var chnDone chan struct{}
	if chnDone, ok = state.mdone.Load(data.Key); !ok {
		return fmt.Errorf("app not found (done property) %v", data.Key)
	}

	log.Debug("[GronosMessage] [TerminateShutdown] terminate shutdown waiting real done asynchronously", data.Key)

	go func() {

		<-chnDone
		log.Debug("[GronosMessage] [TerminateShutdown] terminate shutdown done", data.Key, ok)

		if alive, ok = state.mali.Load(data.Key); !ok {
			log.Debug("[GronosMessage] [TerminateShutdown] app not found (alive property)", data.Key)
			future.PublishError(fmt.Errorf("app not found (alive property) %v", data.Key))
			switch value := g.getSystemMetadata().(type) {
			case Success[*Metadata[K]]:
				g.enqueue(ChannelTypePublic, value.Value, NewMessageRuntimeError(data.Key, fmt.Errorf("app not found (alive property) %v", data.Key)))
			case Failure:
				// TODO: we need a custom log system or just use slog for being able to process it
				log.Error("[GronosMessage] [TerminateShutdown] failed to send runtime error", value.Err)
				return
			}
			return
		}

		if alive {
			state.mali.Store(data.Key, false)
		}

		state.mstatus.Store(data.Key, StatusShutdownTerminated)

		log.Debug("[GronosMessage] [TerminateShutdown] terminate shutdown terminated", data.Key)

		future.Close()
	}()

	return nil
}

func (g *gronos[K]) handlePanicShutdown(
	state *gronosState[K],
	metadata *Metadata[K],
	data struct {
		KeyMessage[K]
		Error error
	},
	future Future[Void],
) error {

	var alive bool
	var ok bool
	if alive, ok = state.mali.Load(data.Key); !ok {
		return fmt.Errorf("app not found (alive property) %v", data.Key)
	}
	if !alive {
		return fmt.Errorf("app already dead %v", data.Key)
	}

	log.Debug("[GronosMessage] [PanickedShutdown] panic", data.Key, data.Error)
	state.mrea.Store(data.Key, data.Error)
	state.mstatus.Store(data.Key, StatusShutdownPanicked)
	if alive {
		state.mali.Store(data.Key, false)
	}

	switch value := g.getSystemMetadata().(type) {
	case Success[*Metadata[K]]:
		g.enqueue(ChannelTypePublic, value.Value, NewMessageRuntimeError(data.Key, data.Error))
	case Failure:
		future.PublishError(value.Err)
		return value.Err
	}

	return nil
}

func (g *gronos[K]) handleErrorShutdown(state *gronosState[K], metadata *Metadata[K], data struct {
	KeyMessage[K]
	Error error
}, future Future[Void]) error {
	var alive bool
	var ok bool
	if alive, ok = state.mali.Load(data.Key); !ok {
		future.PublishError(fmt.Errorf("app not found (alive property) %v", data.Key))
		return fmt.Errorf("app not found (alive property) %v", data.Key)
	}
	if !alive {
		future.PublishError(fmt.Errorf("app already dead %v", data.Key))
		return fmt.Errorf("app already dead %v", data.Key)
	}

	log.Debug("[GronosMessage] [ErroredShutdown] error", data.Key, data.Error)
	state.mrea.Store(data.Key, data.Error)
	state.mstatus.Store(data.Key, StatusShutdownError)
	if alive {
		state.mali.Store(data.Key, false)
	}

	switch value := g.getSystemMetadata().(type) {
	case Success[*Metadata[K]]:
		g.enqueue(ChannelTypePublic, value.Value, NewMessageRuntimeError(data.Key, data.Error))
	case Failure:
		future.PublishError(value.Err)
		return value.Err
	}

	future.Close()

	return nil
}

func (g *gronos[K]) handleRuntimeApplication(state *gronosState[K], metadata *Metadata[K], key K) {
	var retries uint
	var shutdown chan struct{}
	var app LifecyleFunc
	var ctx context.Context

	// Load necessary data
	if value, ok := state.mret.Load(key); ok {
		retries = value
	}
	if value, ok := state.mapp.Load(key); ok {
		app = value
	}
	if value, ok := state.mctx.Load(key); ok {
		ctx = value
	}
	if value, ok := state.mshu.Load(key); ok {
		shutdown = value
	}

	ctx = context.WithValue(ctx, keyKey, key)

	state.mstatus.Store(key, StatusRunning)

	errChan := make(chan error, 1)
	defer close(errChan)

	// TODO: add priority for shutdown as a weight
	// vertex := gograph.NewVertex(key, gograph.WithVertexWeight(1))
	// state.graph.AddVertex(vertex)
	var err error
	var vertex string

	if vertex, err = state.graph.AddVertex(NewLifecycleVertexData(key)); err != nil {
		// TODO: log error on cerr
		log.Error("[RuntimeApplication] failed to add vertex", key, err)
		return
	}

	log.Debug("[RuntimeApplication] goroutine executed", "key", key, "metadata", metadata.String(), "vertex", vertex)

	if metadata.HasKey() {
		fmt.Println("metadata.HasKey()", state.rootVertex, vertex)
		if metadata.GetKey() == g.computedRootKey {
			if err = state.graph.AddEdge(state.rootVertex, vertex); err != nil {
				// TODO: log error on cerr
				log.Error("[RuntimeApplication] failed to add edge", state.rootVertex, vertex, err)
				return
			}
		} else {
			parent := metadata.GetKeyString()
			fmt.Println("\tparent", parent, "of", vertex)
			if err = state.graph.AddEdge(parent, vertex); err != nil {
				// TODO: log error on cerr
				log.Error("[RuntimeApplication] failed to add edge", parent, vertex, err)
				return
			}
		}
	} else {
		// TODO: i think that a bug, come back later to think about it
	}

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

	// when the runtime application is REALLY done, which mean the closer or cancel or panic was called or returned an error
	defer func() {
		log.Debug("[RuntimeApplication] defer", key, err)
		if chndone, ok := state.mdone.Load(key); ok {
			close(chndone)
			log.Debug("[RuntimeApplication] defer closed done channel", key)
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
	if value, ok := state.mali.Load(key); !ok || !value {
		return
	}

	log.Debug("[RuntimeApplication] com", key, err)

	switch value := g.getSystemMetadata().(type) {
	case Success[*Metadata[K]]:
		//	Notify to the global state
		if err != nil {
			if errors.Is(err, context.Canceled) {
				log.Debug("[RuntimeApplication] com canceled", key, err)
				g.enqueue(ChannelTypePublic, value.Value, NewMessageCancelledShutdown(key, err))
			} else if errors.Is(err, ErrPanic) {
				log.Debug("[RuntimeApplication] com panic", key, err)
				g.enqueue(ChannelTypePublic, value.Value, NewMessagePanickedShutdown(key, err))
			} else {
				log.Debug("[RuntimeApplication] com error", key, err)
				g.enqueue(ChannelTypePublic, value.Value, NewMessageErroredShutdown(key, err))
			}
		} else {
			log.Debug("[RuntimeApplication] com terminate", key)
			g.enqueue(ChannelTypePublic, value.Value, NewMessageTerminatedShutdown(key))
		}
	case Failure:
		log.Error("[RuntimeApplication] failed to get system metadata and notify end of application", value.Err)
		// TODO: can we have a backup solution?
	}

}
