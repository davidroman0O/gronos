package gronos

import "context"

type playPauseContext struct {
	context.Context
	done   chan struct{}
	pause  chan struct{}
	resume chan struct{}
}

type Pause <-chan struct{}
type Play <-chan struct{}

var getIDKey gronosKey = gronosKey("getID")
var pauseKey gronosKey = gronosKey("pause")

func WithPlayPause(parent context.Context) (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(parent)
	done := make(chan struct{})
	pause := make(chan struct{}, 1)
	resume := make(chan struct{}, 1)

	playPauseCtx := &playPauseContext{ctx, done, pause, resume}
	ctx = context.WithValue(ctx, pauseKey, playPauseCtx)

	return ctx, cancel
}

func Paused(ctx context.Context) (Pause, Play, bool) {
	playPauseCtx, ok := ctx.Value(pauseKey).(*playPauseContext)
	if !ok {
		return nil, nil, false
	}
	return Pause(playPauseCtx.pause), Play(playPauseCtx.resume), true
}

func PlayPauseOperations(ctx context.Context) (func(), func(), bool) {
	playPauseCtx, ok := ctx.Value(pauseKey).(*playPauseContext)
	if !ok {
		return nil, nil, false
	}

	pauseFunc := func() {
		select {
		case playPauseCtx.pause <- struct{}{}:
		default:
		}
	}

	resumeFunc := func() {
		select {
		case playPauseCtx.resume <- struct{}{}:
		default:
		}
	}

	return pauseFunc, resumeFunc, true
}
