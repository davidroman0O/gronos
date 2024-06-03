package gronos

import "context"

type shutdownContext struct {
	shutdown *Signal
}

var shutdownKey gronosKey = gronosKey("shutdown")

func withShutdown(parent context.Context) context.Context {
	ctx := context.WithValue(parent, shutdownKey, shutdownContext{
		shutdown: newSignal(),
	})
	return ctx
}

func UseShutdown(ctx context.Context) (shutdown *Signal, ok bool) {
	signalCtx, ok := ctx.Value(shutdownKey).(shutdownContext)
	if !ok {
		return nil, false
	}
	return signalCtx.shutdown, true
}
