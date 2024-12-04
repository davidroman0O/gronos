package gronos

import (
	"context"
	"fmt"
)

// UseBus retrieves the communication channel from a context created by gronos.
func UseBus(ctx context.Context) (func(m Message) bool, error) {
	value := ctx.Value(comKey)
	if value == nil {
		return nil, fmt.Errorf("com not found in context")
	}
	return value.(func(m Message) bool), nil
}

func UseBusWait(ctx context.Context) (func(fn FnWait) <-chan struct{}, error) {
	value := ctx.Value(comKeyWait)
	if value == nil {
		return nil, fmt.Errorf("com not found in context")
	}
	return value.(func(fn FnWait) <-chan struct{}), nil
}

func UseBusConfirm(ctx context.Context) (func(fn FnConfirm) <-chan bool, error) {
	value := ctx.Value(comKeyConfirm)
	if value == nil {
		return nil, fmt.Errorf("com not found in context")
	}
	return value.(func(fn FnConfirm) <-chan bool), nil
}
