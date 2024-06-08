package events

import "time"

type Event[T any] struct {
	acknowledged   bool   // ack
	notAckowledged bool   // nack
	created        int64  // timestamp
	correlationID  string // unique identifier
	returnAddress  string // original/final requester
	replyTo        string // current requester
	metadata       map[string]string
	payload        T // represent a user-defined message
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
		created: time.Now().UnixNano(),
		payload: payload,
	}
	for _, opt := range opts {
		opt(e)
	}
	return e
}
