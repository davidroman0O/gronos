package gronos

import (
	"context"

	"github.com/davidroman0O/gronos/ringbuffer"
)

type mailboxContext struct {
	buffer *ringbuffer.RingBuffer[message]
}

var mailboxKey gronosKey = gronosKey("mailbox")

func withMailbox(parent context.Context) context.Context {
	ctx := context.WithValue(parent, mailboxKey, mailboxContext{
		buffer: ringbuffer.New[message](
			ringbuffer.WithInitialSize(300),
			ringbuffer.WithExpandable(true),
		), // todo add parameters
	})
	return ctx
}

// Check messages when they are available
func UseMailbox(ctx context.Context) (<-chan []message, bool) {
	mailboxCtx, ok := ctx.Value(mailboxKey).(mailboxContext)
	if !ok {
		return nil, false
	}
	return mailboxCtx.buffer.DataAvailable(), true
}

// Private mailbox with full control, which is not part of the developer experience and not part of the control flow.
func useMailbox(ctx context.Context) (*ringbuffer.RingBuffer[message], bool) {
	mailboxCtx, ok := ctx.Value(mailboxKey).(mailboxContext)
	if !ok {
		return nil, false
	}
	return mailboxCtx.buffer, true
}
