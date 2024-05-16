package gronos

type Payload interface{}
type Metadata map[string]interface{}

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

type messageOption func(*message) error

func (m *message) applyOptions(options ...messageOption) error {
	for _, option := range options {
		if err := option(m); err != nil {
			return err
		}
	}
	return nil
}

func WitName(name string) messageOption {
	return func(m *message) error {
		m.name = name
		return nil
	}
}

func WithID(id uint) messageOption {
	return func(m *message) error {
		m.to = id
		return nil
	}
}

func NewMessage(payload interface{}, opts ...messageOption) (*message, error) {
	m := message{
		to:       0,
		name:     "",
		Payload:  payload,
		Metadata: make(map[string]interface{}),
		ack:      make(chan struct{}),
		noAck:    make(chan struct{}),
	}
	if err := m.applyOptions(opts...); err != nil {
		return nil, err
	}
	return &m, nil
}

func withGronos() messageOption {
	return func(m *message) error {
		m.Metadata["$_gronos"] = "" // just key exists to say we tagged it
		m.Metadata["$_gronos::retries"] = 0
		return nil
	}
}
