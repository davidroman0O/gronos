package gronos

import "fmt"

// Courier is responsible for delivering error messages. When an error occurs, it's like the Courier "picks up" the error and "delivers" to the central mail station.
// It is a simple intermediary.
type Courier struct {
	closed bool
	c      chan error
	e      chan envelope
	es     chan []envelope
}

func (s *Courier) readNotices() <-chan error {
	return s.c
}

func (s *Courier) readMails() <-chan envelope {
	return s.e
}

func (s *Courier) Deliver(env envelope) {
	if s.closed {
		fmt.Println("Courier closed")
		return
	}
	if env.Msg != nil {
		s.e <- env
	}
}

func (s *Courier) DeliverMany(envs []envelope) {
	if s.closed {
		fmt.Println("Courier closed")
		return
	}
	if envs != nil {
		s.es <- envs
	}
}

func (s *Courier) Transmit(err error) {
	if s.closed {
		// slog.Info("Courier closed")
		return
	}
	if err != nil {
		s.c <- err
	}
}

// todo check for unecessary close
func (s *Courier) Complete() {
	if s.closed {
		return
	}
	s.closed = true
	close(s.c)
	close(s.e)
	close(s.es)
}

func newCourier() *Courier {
	c := make(chan error, 1)    // should be 3 modes: unbuffered, buffered, with 1 buffered channel to prevent panic on multiple ctrl+c signals
	e := make(chan envelope, 1) // should be 3 modes: unbuffered, buffered, with 1 buffered channel to prevent panic on multiple ctrl+c signals
	es := make(chan []envelope, 1)
	return &Courier{
		closed: false,
		c:      c,
		e:      e,
		es:     es,
	}
}
