package gronos

import "context"

type MessageType string

type PortKey string

func (p PortKey) String() string {
	return string(p)
}

type Metadata map[string]interface{}

type Payload interface{}

type Message struct {
	// messages will be exchanged mostly at runtime
	// could be useful to have a context to add timeouts, deadlines, etc
	context.Context
	metadata        Metadata
	payload         Payload
	ackowledged     <-chan struct{}
	notacknowledged <-chan struct{}
}

func NewMessage(ctx context.Context, metadata Metadata, payload Payload) *Message {
	return &Message{
		Context:  ctx,
		metadata: metadata,
		payload:  payload,
		// ackowledged:     make(<-chan struct{}),
		// notacknowledged: make(<-chan struct{}),
	}
}

type metadataOption func(*Metadata)

func WithTo(to PortKey) metadataOption {
	return func(m *Metadata) {
		(*m)["to"] = to
	}
}

func WithReplyTo(replyTo string) metadataOption {
	return func(m *Metadata) {
		(*m)["replyTo"] = replyTo
	}
}

func WithReturnTo(returnTo string) metadataOption {
	return func(m *Metadata) {
		(*m)["returnTo"] = returnTo
	}
}

func NewMetadata(opts ...metadataOption) Metadata {
	m := Metadata{}
	for _, opt := range opts {
		opt(&m)
	}
	return m
}
