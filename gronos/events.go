package gronos

import (
	"sync"
	"time"
)

// thank you `watermill` for the inspiration
var closedchan = make(chan struct{})

func init() {
	close(closedchan)
}

type ackType int

const (
	noAckSent ackType = iota
	ack
	nack
)

type Event[T any] struct {
	ackMutex    sync.Mutex
	ackSentType ackType

	// On those two events, we need to have a way to wrap it
	ack   chan struct{} // ack
	noAck chan struct{} // nack

	created       int64  // timestamp
	correlationID string // unique identifier
	returnAddress string // original/final requester
	replyTo       string // current requester
	metadata      map[string]string
	payload       T // represent a user-defined message
}

func (e *Event[T]) Ack() bool {
	e.ackMutex.Lock()
	defer e.ackMutex.Unlock()

	if e.ackSentType == nack {
		return false
	}
	if e.ackSentType != noAckSent {
		return true
	}

	e.ackSentType = ack
	if e.ack == nil {
		e.ack = closedchan
	} else {
		close(e.ack)
	}

	return true
}

func (e *Event[T]) Nack() bool {
	e.ackMutex.Lock()
	defer e.ackMutex.Unlock()

	if e.ackSentType == ack {
		return false
	}
	if e.ackSentType != noAckSent {
		return true
	}

	e.ackSentType = nack

	if e.noAck == nil {
		e.noAck = closedchan
	} else {
		close(e.noAck)
	}

	return true
}

type EventOption[T any] func(*Event[T])

func WithCorrelationID[T any](correlationID string) EventOption[T] {
	return func(e *Event[T]) {
		e.correlationID = correlationID
	}
}

func WithReturnAddress[T any](returnAddress string) EventOption[T] {
	return func(e *Event[T]) {
		e.returnAddress = returnAddress
	}
}

func WithReplyTo[T any](replyTo string) EventOption[T] {
	return func(e *Event[T]) {
		e.replyTo = replyTo
	}
}

func WithMetadata[T any](metadata map[string]string) EventOption[T] {
	return func(e *Event[T]) {
		e.metadata = metadata
	}
}

func FromMetadata[T any](metadata map[string]string) EventOption[T] {
	return func(e *Event[T]) {
		if value, ok := e.metadata["correlationID"]; ok {
			e.correlationID = value
		}
		if value, ok := e.metadata["returnAddress"]; ok {
			e.returnAddress = value
		}
		if value, ok := e.metadata["replyTo"]; ok {
			e.replyTo = value
		}
	}
}

func NewEvent[T any](payload T, opts ...EventOption[T]) *Event[T] {
	e := &Event[T]{
		created:  time.Now().UnixNano(),
		metadata: make(map[string]string),
		payload:  payload,
		ack:      make(chan struct{}),
		noAck:    make(chan struct{}),
	}
	for _, opt := range opts {
		opt(e)
	}
	return e
}
