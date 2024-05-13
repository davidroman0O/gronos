package gronos

type Signal struct {
	closed bool
	c      chan struct{}
}

func (s *Signal) Complete() {
	if s.closed {
		return
	}
	close(s.c)
	s.closed = true
}

func (s *Signal) Await() <-chan struct{} {
	return s.c
}

func newSignal() *Signal {
	c := make(chan struct{})
	return &Signal{
		c: c,
	}
}
