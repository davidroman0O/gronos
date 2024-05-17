package gronos

import "context"

type mailboxContext struct {
	buffer *RingBuffer[message]
}

var mailboxKey gronosKey = gronosKey("mailbox")

func WithMailbox(parent context.Context) context.Context {
	ctx := context.WithValue(parent, mailboxKey, mailboxContext{
		buffer: NewRingBuffer[message](
			WithInitialSize(300),
			WithExpandable(true),
		), // todo add parameters
	})
	return ctx
}

func Messages(ctx context.Context) (<-chan []message, bool) {
	mailboxCtx, ok := ctx.Value(mailboxKey).(mailboxContext)
	if !ok {
		return nil, false
	}
	return mailboxCtx.buffer.DataAvailable(), true
}
