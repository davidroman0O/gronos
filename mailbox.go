package gronos

// It's a Mailbox
type Mailbox struct {
	closed bool
	// r      chan envelope
	r *RingBuffer[envelope]
	// TODO: add optional circular buffer
}

func (s *Mailbox) Read() <-chan envelope {
	out := make(chan envelope)
	go func() {
		defer close(out)
		for msg := range s.r.GetDataAvailableChannel() {
			for _, v := range msg {
				out <- v
			}
		}
	}()
	return out
}

func (r *Mailbox) post(msg envelope) {
	r.r.Push(msg)
}

// todo check for unecessary close
func (s *Mailbox) Complete() {
	if s.closed {
		return
	}
	s.r.Close()
	s.closed = true
}

type MailboxConfig struct {
	initialSize int
	expandable  bool
	throughput  int
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

func MailboxWithThroughput(throughput int) MailboxOption {
	return func(c *MailboxConfig) {
		c.throughput = throughput
	}
}

func newMailbox(opts ...MailboxOption) *Mailbox {
	c := &MailboxConfig{}
	for i := 0; i < len(opts); i++ {
		opts[i](c)
	}
	m := &Mailbox{
		r: NewRingBuffer[envelope](c.initialSize, c.expandable, c.throughput),
	}
	return m
}
