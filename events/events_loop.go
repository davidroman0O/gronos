package events

import (
	"github.com/davidroman0O/gronos/clock"
	"github.com/davidroman0O/gronos/ringbuffer"
)

// Provide a registry that knows what goes in and out for each ports.
// Should provide a router with in/out capabilities for each port.
// TODO: mean that each port is connected to something which have a name too like an app? runtime?

type MessageType string

type Loop[Key comparable] struct {
	ports  map[Key]Port[Key] // We only have a finite amount of gateway that will have a finite amount of message types.
	buffer ringbuffer.RingBuffer[interface{}]
	clock  *clock.Clock
}

func New[Key comparable](gateways ...Port[Key]) *Loop[Key] {
	r := &Loop[Key]{
		ports: make(map[Key]Port[Key]),
	}
	for _, g := range gateways {
		r.ports[g.gateway] = g
	}
	return r
}

func (r *Loop[Key]) Tick() {

}

func (r *Loop[Key]) Add(g Port[Key]) {
	r.ports[g.gateway] = g
}
