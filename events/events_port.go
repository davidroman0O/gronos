package events

import (
	"reflect"

	"github.com/davidroman0O/gronos/clock"
)

// Verify that received messages are of the correct type
type Port[Key comparable] struct {
	gateway Key
	inout   map[MessageType]reflect.Type
	clock   *clock.Clock
}

type registration func() (MessageType, reflect.Type)

func Register[T any]() registration {
	return func() (MessageType, reflect.Type) {
		return MessageType(reflect.TypeFor[T]().Name()), reflect.TypeFor[T]()
	}
}

type GatewayOption[Key comparable] func(*Port[Key])

func NewPort[Key comparable](gateway Key, fn ...registration) Port[Key] {
	g := Port[Key]{
		gateway: gateway,
		inout:   make(map[MessageType]reflect.Type),
	}

	for _, opt := range fn {
		key, typ := opt()
		g.inout[key] = typ
	}

	return g
}

func (r *Port[Key]) Tick() {

}
