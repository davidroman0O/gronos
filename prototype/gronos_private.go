package gronos

import (
	"sync"

	"github.com/charmbracelet/log"
)

type MessageRequestMetadata[K Primitive] struct {
	FutureMessage[struct{}, *Metadata[K]]
}

func NewMessageRequestMetadata[K Primitive]() *MessageRequestMetadata[K] {
	return &MessageRequestMetadata[K]{
		FutureMessage: FutureMessage[struct{}, *Metadata[K]]{
			Data:   struct{}{},
			Result: NewFuture[*Metadata[K]](),
		},
	}
}

type MessageRequestPayload[K Primitive] struct {
	FutureMessage[struct{}, *MessagePayload[K]]
}

func NewMessageRequestPayload[K Primitive]() *MessageRequestPayload[K] {
	return &MessageRequestPayload[K]{
		FutureMessage: FutureMessage[struct{}, *MessagePayload[K]]{
			Data:   struct{}{},
			Result: NewFuture[*MessagePayload[K]](),
		},
	}
}

type MessageReleaseMetadata[K Primitive] struct {
	FutureMessage[*Metadata[K], Void]
}

func NewMessageReleaseMetadata[K Primitive](m *Metadata[K]) *MessageReleaseMetadata[K] {
	return &MessageReleaseMetadata[K]{
		FutureMessage: FutureMessage[*Metadata[K], Void]{
			Data:   m,
			Result: NewFuture[Void](),
		},
	}
}

type MessageReleasePayload[K Primitive] struct {
	FutureMessage[*MessagePayload[K], Void]
}

func NewMessageReleasePayload[K Primitive](m *MessagePayload[K]) *MessageReleasePayload[K] {
	return &MessageReleasePayload[K]{
		FutureMessage: FutureMessage[*MessagePayload[K], Void]{
			Data:   m,
			Result: NewFuture[Void](),
		},
	}
}

func (g *gronos[K]) internals(errChan chan<- error) {

	var metadataPool = sync.Pool{
		New: func() interface{} {
			return NewMetadata[K]()
		},
	}

	var payloadPool = sync.Pool{
		New: func() interface{} {
			return &MessagePayload[K]{
				Metadata: nil,
				Message:  nil,
			}
		},
	}

	for m := range g.privateChn {
		switch msg := m.(type) {
		case *MessageRequestMetadata[K]:
			log.Debug("[GronosInternals] [MessageRequestMetadata]")
			metadata := metadataPool.Get().(*Metadata[K])
			msg.Result <- SuccessResult(metadata)
			close(msg.Result)

		case *MessageRequestPayload[K]:
			log.Debug("[GronosInternals] [MessageRequestPayload]")
			payload := payloadPool.Get().(*MessagePayload[K])
			msg.Result <- SuccessResult(payload)
			close(msg.Result)

		case *MessageReleaseMetadata[K]:
			log.Debug("[GronosInternals] [MessageReleaseMetadata]")
			metadataPool.Put(msg.Data)
			close(msg.Result)

		case *MessageReleasePayload[K]:
			log.Debug("[GronosInternals] [MessageReleasePayload]")
			payloadPool.Put(msg.Data)
			payloadPool.Put(msg.Data)
			close(msg.Result)
		}
	}

	log.Debug("[Gronos] Internal channel closed")
}
