package gronos

import (
	"sync"

	"github.com/charmbracelet/log"
)

type requestMetadata[K Primitive] struct {
	Response Future[*Metadata[K]]
}

type requestPayload[K Primitive] struct {
	Response Future[*MessagePayload[K]]
}

type releaseMetadata[K Primitive] struct {
	Metadata *Metadata[K]
	Response FutureVoid
}

type releasePayload[K Primitive] struct {
	Payload  *MessagePayload[K]
	Response FutureVoid
}

func MsgRequestMetadata[K Primitive]() (Future[*Metadata[K]], *requestMetadata[K]) {
	response := make(Future[*Metadata[K]], 1)
	msg := &requestMetadata[K]{Response: response}
	return response, msg
}

func MsgRequestPayload[K Primitive]() (Future[*MessagePayload[K]], *requestPayload[K]) {
	response := make(Future[*MessagePayload[K]], 1)
	msg := &requestPayload[K]{Response: response}
	return response, msg
}

func MsgReleaseMetadata[K Primitive](m *Metadata[K]) (FutureVoid, *releaseMetadata[K]) {
	response := make(FutureVoid, 1)
	msg := &releaseMetadata[K]{Metadata: m, Response: response}
	return response, msg
}

func MsgReleasePayload[K Primitive](m *MessagePayload[K]) (FutureVoid, *releasePayload[K]) {
	response := make(FutureVoid, 1)
	msg := &releasePayload[K]{Payload: m, Response: response}
	return response, msg
}

func (g *gronos[K]) internals(errChan chan<- error) {

	var metadataPool = sync.Pool{
		New: func() interface{} {
			return NewMetadata[K]()
		},
	}

	var messagePayloadPool = sync.Pool{
		New: func() interface{} {
			return &MessagePayload[K]{
				Metadata: nil,
				Message:  nil,
			}
		},
	}

	for m := range g.privateChn {
		switch msg := m.(type) {
		case *requestMetadata[K]:
			log.Debug("[GronosInternals] [RequestMetadata]")
			metadata := metadataPool.Get().(*Metadata[K])
			msg.Response <- SuccessResult(metadata)
			close(msg.Response)

		case *requestPayload[K]:
			log.Debug("[GronosInternals] [RequestPayload]")
			payload := messagePayloadPool.Get().(*MessagePayload[K])
			msg.Response <- SuccessResult(payload)
			close(msg.Response)

		case *releaseMetadata[K]:
			log.Debug("[GronosInternals] [ReleaseMetadata]")
			metadataPool.Put(msg.Metadata)
			msg.Response <- nil
			close(msg.Response)

		case *releasePayload[K]:
			log.Debug("[GronosInternals] [ReleasePayload]")
			messagePayloadPool.Put(msg.Payload)
			msg.Response <- nil
			close(msg.Response)
		}
	}

	log.Debug("[Gronos] Internal channel closed")
}
