package gronos

import (
	"context"
)

// ExtensionHooks defines the interface for Gronos extensions
type ExtensionHooks[K comparable] interface {
	OnStart(ctx context.Context, errChan chan<- error) error
	OnStop(ctx context.Context, errChan chan<- error) error
	OnNewRuntime(ctx context.Context) context.Context
	OnStopRuntime(ctx context.Context) context.Context
	OnMsg(ctx context.Context, m Message) error
}
