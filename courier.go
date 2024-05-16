package gronos

import (
	"fmt"
	"log/slog"
)

// Courier is responsible for delivering error messages. When an error occurs, it's like the Courier "picks up" the error and "delivers" to the central mail station.
// It is a simple intermediary.
type Courier struct {
	closed bool
	c      chan error
	e      chan message
	es     chan []message
}

func (s *Courier) readNotices() <-chan error {
	return s.c
}

func (s *Courier) readMails() <-chan message {
	return s.e
}

func (s *Courier) Deliver(env message) {
	if s.closed {
		fmt.Println("Courier closed")
		return
	}
	if env.Payload != nil {
		slog.Info("Courier delivering", slog.Any("env", env))
		s.e <- env
	}
}

func (s *Courier) DeliverMany(envs []message) {
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

// TODO: should configure it
func newCourier() *Courier {
	c := make(chan error, 1)   // should be 3 modes: unbuffered, buffered, with 1 buffered channel to prevent panic on multiple ctrl+c signals
	e := make(chan message, 1) // should be 3 modes: unbuffered, buffered, with 1 buffered channel to prevent panic on multiple ctrl+c signals
	es := make(chan []message, 1)
	return &Courier{
		closed: false,
		c:      c,
		e:      e,
		es:     es,
	}
}
