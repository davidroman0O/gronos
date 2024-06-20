package gronos

import (
	"context"
	"reflect"

	"github.com/davidroman0O/gronos/clock"
	"github.com/davidroman0O/gronos/valueobjects"
)

// Verify that received messages are of the correct type
type Port[Key comparable] struct {
	portKey Key
	inout   map[valueobjects.MessageType]reflect.Type
	clock   *clock.Clock
}

type registration func() (valueobjects.MessageType, reflect.Type)

func Register[T any]() registration {
	return func() (valueobjects.MessageType, reflect.Type) {
		return valueobjects.MessageType(reflect.TypeFor[T]().Name()), reflect.TypeFor[T]()
	}
}

type GatewayOption[Key comparable] func(*Port[Key])

func NewPort[Key comparable](port Key, fn ...registration) Port[Key] {
	g := Port[Key]{
		portKey: port,
		inout:   make(map[valueobjects.MessageType]reflect.Type),
	}

	for _, opt := range fn {
		key, typ := opt()
		g.inout[key] = typ
	}

	return g
}

func (r *Port[Key]) Tick() {

}

func (r *Port[Key]) Push(
	ctx context.Context,
	metadata interface{},
	msg interface{}) error {

	return nil
}
