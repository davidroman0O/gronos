package events

import (
	"sync"
	"time"

	"github.com/davidroman0O/gronos/clock"
	"github.com/davidroman0O/gronos/ringbuffer"
	"github.com/davidroman0O/gronos/valueobjects"
)

// Provide a registry that knows what goes in and out for each ports.
// Should provide a router with in/out capabilities for each port.
// TODO: mean that each port is connected to something which have a name too like an app? runtime?

type Loop[K valueobjects.PortKey] struct {
	syncPorts sync.Map
	buffer    *ringbuffer.RingBuffer[valueobjects.Message]
	clock     *clock.Clock
}

type loopConfig struct {
	clock     *clock.Clock
	syncPorts sync.Map
}

type LoopOption func(*loopConfig)

func WithClock(clock *clock.Clock) LoopOption {
	return func(c *loopConfig) {
		c.clock = clock
	}
}

func WithPort[K valueobjects.PortKey](port Port[K]) LoopOption {
	return func(c *loopConfig) {
		c.syncPorts.Store(port.portKey, port)
	}
}

func New(opts ...LoopOption) (*Loop[valueobjects.PortKey], error) {
	c := loopConfig{
		syncPorts: sync.Map{},
		clock: clock.New(
			clock.WithInterval(1 * time.Millisecond),
		),
	}
	for _, opt := range opts {
		opt(&c)
	}
	r := &Loop[valueobjects.PortKey]{
		syncPorts: sync.Map{},
		clock:     c.clock,
		buffer: ringbuffer.New[valueobjects.Message](
			ringbuffer.WithInitialSize(1000),
			ringbuffer.WithExpandable(true),
		),
	}
	// simply move the data
	c.syncPorts.Range(
		func(key, value any) bool {
			if port, ok := value.(Port[valueobjects.PortKey]); ok {
				r.Store(&port)
			}
			return true
		},
	)
	r.clock.Add(r, clock.ManagedTimeline)
	go r.clock.Start() // TODO: you have to close it
	return r, nil
}

func (l *Loop[Key]) Close() {
	l.clock.Stop()
}

func (l *Loop[Key]) Tick() {
	l.buffer.Tick()
	// msgs := <-l.buffer.DataAvailable()
	// for _, v := range msgs {
	// 	if value, ok := l.syncPorts.Load(v); ok {
	// 		value.(Port[Key]).Handle(v)
	// 	}
	// }
}

func (l *Loop[Key]) Store(g *Port[valueobjects.PortKey]) {
	l.clock.Add(g, clock.ManagedTimeline)
	l.syncPorts.Store(g.portKey, g)
}

func (l *Loop[Key]) Load(key Key) (Port[Key], bool) {
	var value any
	var ok bool
	if value, ok = l.syncPorts.Load(key); !ok {
		return Port[Key]{}, false

	}
	var port Port[Key]
	if port, ok = value.(Port[Key]); ok {
		return port, true
	}
	return Port[Key]{}, false
}
