package gronos

import (
	"context"
)

// Extension defines the interface for Gronos extensions
type Extension[K Primitive] interface {
	OnStart(ctx context.Context, errChan chan<- error) error
	OnStop(ctx context.Context, errChan chan<- error) error
	OnNewRuntime(ctx context.Context) context.Context
	OnStopRuntime(ctx context.Context) context.Context
	OnMsg(ctx context.Context, m *MessagePayload[K]) error
}

type Pool[K Primitive] interface {
	OnMsg(ctx context.Context, m *MessagePayload[K]) error
}
