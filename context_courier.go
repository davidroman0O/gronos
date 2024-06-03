package gronos

import "context"

// The Courier is a proxy to write on the router ringbuffer to be dispatched
type courierContext struct {
	courier *Courier
}

var courierKey gronosKey = gronosKey("courier")

func withCourier(parent context.Context) context.Context {
	ctx := context.WithValue(parent, courierKey, courierContext{
		courier: newCourier(),
	})
	return ctx
}

func UseCourier(ctx context.Context) (courier *Courier, ok bool) {
	signalCtx, ok := ctx.Value(courierKey).(courierContext)
	if !ok {
		return nil, false
	}
	return signalCtx.courier, true
}
