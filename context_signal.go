package gronos

import "context"

type signalContext struct {
	shutdown *Signal
}

var signalKey gronosKey = gronosKey("signal")

func WithSignals(parent context.Context) context.Context {
	ctx := context.WithValue(parent, signalKey, signalContext{
		shutdown: newSignal(),
	})
	return ctx
}

func Signals(ctx context.Context) (shutdown *Signal, ok bool) {
	signalCtx, ok := ctx.Value(signalKey).(signalContext)
	if !ok {
		return nil, false
	}
	return signalCtx.shutdown, true
}
