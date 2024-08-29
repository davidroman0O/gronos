package gronos

import (
	"context"
	"fmt"
)

// UseBus retrieves the communication channel from a context created by gronos.
func UseBus(ctx context.Context) (func(m FutureMessageInterface, opt FutureResultOption) Result, error) {
	value := ctx.Value(ctxCommunicationPublicKey)
	if value == nil {
		return nil, fmt.Errorf("com not found in context")
	}
	return value.(func(m FutureMessageInterface, opt FutureResultOption) Result), nil
}
