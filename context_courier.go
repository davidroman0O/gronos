package gronos

import "context"

type courierContext struct {
	shutdown *Signal
}

var courierKey gronosKey = gronosKey("courier")

func WithCourier(parent context.Context) context.Context {
	ctx := context.WithValue(parent, courierKey, courierContext{
		shutdown: newSignal(),
	})
	return ctx
}

func Courrier(ctx context.Context) (shutdown *Signal, ok bool) {
	signalCtx, ok := ctx.Value(courierKey).(courierContext)
	if !ok {
		return nil, false
	}
	return signalCtx.shutdown, true
}
