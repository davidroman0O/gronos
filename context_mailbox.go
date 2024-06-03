package gronos

import "context"

type mailboxContext struct {
	buffer *RingBuffer[message]
}

var mailboxKey gronosKey = gronosKey("mailbox")

func withMailbox(parent context.Context) context.Context {
	ctx := context.WithValue(parent, mailboxKey, mailboxContext{
		buffer: NewRingBuffer[message](
			WithInitialSize(300),
			WithExpandable(true),
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
func useMailbox(ctx context.Context) (*RingBuffer[message], bool) {
	mailboxCtx, ok := ctx.Value(mailboxKey).(mailboxContext)
	if !ok {
		return nil, false
	}
	return mailboxCtx.buffer, true
}
