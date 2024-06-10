package gronos

import (
	"fmt"
	"log/slog"

	"github.com/davidroman0O/gronos/ringbuffer"
)

// Courier is responsible for delivering error messages. When an error occurs, it's like the Courier "picks up" the error and "delivers" to the central mail station.
// It is a simple intermediary.
// -[x]-[xxx]-[xx]-> [Courier] -[xxxxxx]-> [Mailbox]
type Courier struct {
	closed bool
	buffer *ringbuffer.RingBuffer[message]
}

func (s *Courier) Deliver(env message) {
	if s.closed {
		fmt.Println("Courier closed")
		return
	}
	if env.Payload != nil {
		slog.Info("Courier delivering", slog.Any("env", env))
		s.buffer.Push(env)
	}
}

func (s *Courier) DeliverMany(envs []message) {
	if s.closed {
		fmt.Println("Courier closed")
		return
	}
	if envs != nil {
		for i := 0; i < len(envs); i++ {
			s.buffer.Push(envs[i])
		}
	}
}

// todo check for unecessary close
func (s *Courier) Complete() {
	if s.closed {
		return
	}
	s.closed = true
	s.buffer.Close()
}

// TODO: should configure it
func newCourier() *Courier {
	return &Courier{
		closed: false,
		buffer: ringbuffer.New[message](),
	}
}

// A courier is a proxy to another mailbox, it will accumulate messages before delivering them in batch to the mailbox.
func (c Courier) Tick() {

}
