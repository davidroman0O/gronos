package gronos

import (
	"github.com/davidroman0O/gronos/clock"
	"github.com/davidroman0O/gronos/ringbuffer"
)

// It's a Mailbox
type Mailbox struct {
	closed bool
	// r      chan envelope
	// r *ringBuffer[envelope]
	buffer *ringbuffer.RingBuffer[message]
	// TODO: add optional circular buffer
}

// TODO change the <-chan envelope to <-chan []envelope
func (s *Mailbox) Read() <-chan message {
	out := make(chan message)
	go func() {
		defer close(out)
		for msg := range s.buffer.DataAvailable() {
			for _, v := range msg {
				out <- v
			}
		}
	}()
	return out
}

func (r *Mailbox) Publish(msg message) {
	r.buffer.Push(msg)
}

// todo check for unecessary close
func (s *Mailbox) Complete() {
	if s.closed {
		return
	}
	s.buffer.Close()
	s.closed = true
}

type MailboxConfig struct {
	initialSize int
	expandable  bool
	clock       *clock.Clock
}

type MailboxOption func(*MailboxConfig)

func MailboxWithInitialSize(size int) MailboxOption {
	return func(c *MailboxConfig) {
		c.initialSize = size
	}
}

func MailboxWithExpandable(expandable bool) MailboxOption {
	return func(c *MailboxConfig) {
		c.expandable = expandable
	}
}

// func MailboxWithThroughput(throughput int) MailboxOption {
// 	return func(c *MailboxConfig) {
// 		c.throughput = throughput
// 	}
// }

func newMailbox(opts ...MailboxOption) *Mailbox {
	c := &MailboxConfig{
		initialSize: 300,
		expandable:  true,
	}
	for i := 0; i < len(opts); i++ {
		opts[i](c)
	}
	m := &Mailbox{
		buffer: ringbuffer.New[message](
			ringbuffer.WithInitialSize(c.initialSize),
			ringbuffer.WithExpandable(c.expandable),
		),
	}
	return m
}
