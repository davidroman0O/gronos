package gronos

import "context"

// RuntimeFunc represents a function that runs a runtime.
type RuntimeFunc func(ctx context.Context, mailbox *Mailbox, courrier *Courier, shutdown *Signal) error
