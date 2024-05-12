package gronos

import (
	"context"
	"log/slog"
	"time"
)

// RuntimeFunc represents a function that runs a runtime.
type RuntimeFunc func(ctx context.Context, mailbox *Mailbox, courrier *Courier, shutdown *Signal) error

// One runtime can receive and send messages while performing it's own task
type Runtime struct {
	name    string // easier to reason with
	id      uint
	ctx     context.Context
	runtime RuntimeFunc
	cancel  context.CancelFunc
	courier *Courier
	mailbox *Mailbox
	signal  *Signal
	done    chan struct{}
}

// not sure if i'm going to use the error here
type OptionRuntime func(*Runtime) error

// might delete that tho
func RuntimeWithContext(ctx context.Context) OptionRuntime {
	return func(r *Runtime) error {
		r.ctx = ctx
		return nil
	}
}

func RuntimeWithTimeout(d time.Duration) OptionRuntime {
	return func(r *Runtime) error {
		ctx, cnfn := context.WithTimeout(r.ctx, d)
		r.cancel = cnfn
		r.ctx = ctx
		return nil
	}
}

func RuntimeWithValue(key, value interface{}) OptionRuntime {
	return func(r *Runtime) error {
		r.ctx = context.WithValue(r.ctx, key, value)
		return nil
	}
}

func RuntimeWithDeadline(d time.Time) OptionRuntime {
	return func(r *Runtime) error {
		ctx, cfn := context.WithDeadline(r.ctx, d)
		r.cancel = cfn
		r.ctx = ctx
		return nil
	}
}

func RuntimeWithRuntime(r RuntimeFunc) OptionRuntime {
	return func(rc *Runtime) error {
		rc.runtime = r
		return nil
	}
}

func RuntimeWithID(id uint) OptionRuntime {
	return func(rc *Runtime) error {
		rc.id = id
		return nil
	}
}

func newRuntime(name string, opts ...OptionRuntime) *Runtime {
	r := Runtime{
		name:    name,
		courier: newCourier(),
		mailbox: newMailbox(),
		signal:  newSignal(),
		done:    make(chan struct{}),
	}
	ctx := context.Background()
	ctx = context.WithValue(ctx, getIDKey, r.id)
	r.ctx, r.cancel = WithPlayPause(ctx) // all should support it
	for _, opt := range opts {
		opt(&r)
	}
	return &r
}

type RuntimeCallbacks struct {
	BeforeStart func()
	AfterStart  func()
	BeforeStop  func()
	AfterStop   func()
	Panic       func(recover interface{})
}

type OptionCallbacks func(*RuntimeCallbacks) error

func WithBeforeStart(cb func()) OptionCallbacks {
	return func(r *RuntimeCallbacks) error {
		runtime := r
		runtime.BeforeStart = cb
		return nil
	}
}

func WithAfterStart(cb func()) OptionCallbacks {
	return func(r *RuntimeCallbacks) error {
		runtime := r
		runtime.AfterStart = cb
		return nil
	}
}

func WithBeforeStop(cb func()) OptionCallbacks {
	return func(r *RuntimeCallbacks) error {
		runtime := r
		runtime.BeforeStop = cb
		return nil
	}
}

func WithAfterStop(cb func()) OptionCallbacks {
	return func(r *RuntimeCallbacks) error {
		runtime := r
		runtime.AfterStop = cb
		return nil
	}
}

func WithPanic(cb func(recover interface{})) OptionCallbacks {
	return func(r *RuntimeCallbacks) error {
		runtime := r
		runtime.Panic = cb
		return nil
	}
}

// Owning the start of the runtime
// I moved that piece of code countless amount of time before figuring out that i had to group it all under a Runtime struct to make the code lighter to read and easier to control
func (r *Runtime) Start(opts ...OptionCallbacks) *Signal {
	cbs := &RuntimeCallbacks{}
	for _, opt := range opts {
		opt(cbs)
	}

	signal := newSignal()
	go func() {
		defer func() {
			// might panic
			if recov := recover(); recov != nil {
				if cbs.Panic != nil {
					cbs.Panic(recov)
				}
			} else {
				if cbs.AfterStop != nil {
					cbs.AfterStop()
				}
			}
			close(r.done) // close the done channel
		}()
		if cbs.BeforeStart != nil {
			cbs.BeforeStart()
		}
		signal.Complete() // signal the start
		if err := r.runtime(r.ctx, r.mailbox, r.courier, r.signal); err != nil {
			slog.Error("Gronos runtime error", slog.Any("id", r.id), slog.Any("error", err))
		}
		if cbs.BeforeStop != nil {
			cbs.BeforeStop()
		}
	}()
	return signal
}

// Trigger it's cancel context function
func (r *Runtime) Cancel() {
	r.cancel() // interruption trigger, not real shutdown, mught be trigger due to take too much time or else
}

// Gracefully shutdon the runtime
func (r *Runtime) GracefulShutdown() <-chan struct{} {
	r.signal.Complete() // trigger real end, application stopping
	return r.done       // waiting for the real end
}
