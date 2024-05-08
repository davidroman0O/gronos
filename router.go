package gronos

import (
	"context"
	"fmt"
	"log/slog"
	"time"
)

type runtimeState uint

const (
	added runtimeState = iota
	started
	stopped
	failed
	panicked
)

// Let's reduce the amount of code that manage messages and have a router that allocate one ringbuffer while analyzing the messages
// - plug runtime functions
// - exchange msg between runtime functions
// - analyze metadata in case it's our own messages
// - collect metrics
// The router shouldn't take too much resources or at best you should be able to specify the resources it can take
// A router is targeting Mailboxes and not runtime functions directly
type Router struct {
	mailbox       *ringBuffer[envelope] // we will get rid of all Mailbox thing and use ringBuffers instead
	runtimesNames *safeMapPtr[string, uint]
	runtimes      *safeMapPtr[uint, RuntimeStation]
	states        *safeMap[uint, runtimeState]

	ctx      context.Context
	ctxFn    context.CancelFunc
	shutdown *Signal
}

type RouterConfig struct {
	initialSize int
	expandable  bool
	throughput  int
}

type RouterOption func(*RouterConfig)

func RouterWithInitialSize(size int) RouterOption {
	return func(c *RouterConfig) {
		c.initialSize = size
	}
}

func RouterWithExpandable(expandable bool) RouterOption {
	return func(c *RouterConfig) {
		c.expandable = expandable
	}
}

func RouterWithThroughput(throughput int) RouterOption {
	return func(c *RouterConfig) {
		c.throughput = throughput
	}
}

func newRouter(opts ...RouterOption) *Router {
	config := &RouterConfig{
		initialSize: 1024,
		expandable:  true,
		throughput:  64,
	}
	for _, v := range opts {
		v(config)
	}
	r := &Router{
		mailbox:       newRingBuffer[envelope](config.initialSize, config.expandable, config.throughput),
		runtimesNames: newSafeMapPtr[string, uint](),
		runtimes:      newSafeMapPtr[uint, RuntimeStation](),
		states:        newSafeMap[uint, runtimeState](),
		shutdown:      newSignal(),
	}
	r.ctx, r.ctxFn = context.WithCancel(context.Background())
	go r.run()
	return r
}

func (r *Router) Add(runtime RuntimeStation) {
	r.runtimesNames.Set(runtime.name, &runtime.id)
	r.runtimes.Set(runtime.id, &runtime)
	r.states.Set(runtime.id, added)
}

func (r *Router) Close() {
	r.ctxFn()
	r.mailbox.Close()
}

func (r *Router) run() {
	ticker := time.NewTicker(time.Millisecond * 100)
	for {
		select {
		case <-r.ctx.Done():

		case <-r.shutdown.Await():

		// when we receive messages, then process them.... bruh
		case msgs := <-r.mailbox.GetDataAvailableChannel():
			fmt.Println(msgs)

		case <-ticker.C:
			// check the status of all runtimes
			if err := r.runtimes.ForEach(func(k uint, v *RuntimeStation) error {
				var state runtimeState
				var err error
				var ok bool
				if state, ok = r.states.Get(k); !ok {
					return fmt.Errorf("runtime %d not found", k)
				}

				switch state {
				case added:
					r.states.Set(k, started)
					go r.runtime(k, v) // will change itself to failed or panicked
				case started:
				case stopped:
				case failed:
				case panicked:
				default:
					//
				}

				return err
			}); err != nil {
				fmt.Println(err)
			}
		}
	}
}

// `goroutine` when starting the runtime
func (r *Router) runtime(id uint, run *RuntimeStation) {
	// it's already a goroutine, we just manage the other goroutines from here

	// goroutine for the runtime, we will have to manage it here
	go func() {
		defer func() {
			if re := recover(); r != nil {
				r.states.Set(id, panicked)
				fmt.Println(re)
			}
		}()
		if err := run.runtime(run.ctx, run.mailbox, run.courier, run.signal); err != nil {
			slog.Error("Gronos runtime error", slog.Any("id", run.id), slog.Any("error", err))
		}
		r.states.Set(id, failed)
	}()
}
