package events

import (
	"sync"
	"time"

	"github.com/davidroman0O/gronos/clock"
	"github.com/davidroman0O/gronos/ringbuffer"
)

// Provide a registry that knows what goes in and out for each ports.
// Should provide a router with in/out capabilities for each port.
// TODO: mean that each port is connected to something which have a name too like an app? runtime?

type MessageType string

type Loop[Key comparable] struct {
	syncPorts sync.Map
	// ports     map[Key]Port[Key] // We only have a finite amount of gateway that will have a finite amount of message types.
	buffer ringbuffer.RingBuffer[interface{}]
	clock  *clock.Clock
}

func New[Key comparable](gateways ...Port[Key]) (*Loop[Key], error) {
	r := &Loop[Key]{
		syncPorts: sync.Map{},
		clock: clock.New(
			clock.WithInterval(1 * time.Millisecond),
		),
	}
	for _, g := range gateways {
		r.syncPorts.Store(g.portKey, g)
	}
	r.clock.Add(r, clock.ManagedTimeline)
	go r.clock.Start() // TODO: you have to close it
	return r, nil
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

func (l *Loop[Key]) Store(g Port[Key]) {
	l.syncPorts.Store(g.portKey, g)
}
