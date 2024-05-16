package gronos

type Payload interface{}
type Metadata map[string]string

// Lowest form of message passing dedicated for runtime communication
// Messages might be wrapped as envelopes when going on the network layer
type message struct {
	to       uint
	name     string
	Payload  Payload
	Metadata Metadata
	ack      chan struct{} // when closed, ackowledge is received (watermill inspiration)
	noAck    chan struct{} // when closed, negative ackowledge is received (watermill inspiration)
}

type messageOption func(*message)

func (m *message) applyOptions(options ...messageOption) {
	for _, option := range options {
		option(m)
	}
}

func WitName(name string) messageOption {
	return func(m *message) {
		m.name = name
	}
}

func WithID(id uint) messageOption {
	return func(m *message) {
		m.to = id
	}
}

func NewMessage(metadata map[string]string, payload interface{}) message {
	return message{
		to:       0,
		name:     "",
		Payload:  payload,
		Metadata: metadata,
		ack:      make(chan struct{}),
		noAck:    make(chan struct{}),
	}
}
