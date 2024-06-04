package gronos

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/davidroman0O/gronos/clock"
	"github.com/davidroman0O/gronos/ringbuffer"
)

var (
	ErrMessageMetadataParse = errors.New("error message parsing metadata")
)

// Let's reduce the amount of code that manage messages and have a router that allocate one ringbuffer while analyzing the messages
// - plug runtime functions
// - exchange msg between runtime functions
// - analyze metadata in case it's our own messages
// - collect metrics
// The router shouldn't take too much resources or at best you should be able to specify the resources it can take
// A router is targeting Mailboxes and not runtime functions directly
type Router struct {
	ctx      context.Context
	ctxFn    context.CancelFunc
	shutdown *Signal

	mailbox *ringbuffer.RingBuffer[message] // we will get rid of all Mailbox thing and use ringBuffers instead

	clock *clock.Clock // clock of registry

	registry *RuntimeRegistry

	courier *Courier
}

type RouterConfig struct {
	initialSize int
	expandable  bool
	throughput  int
	timeout     *time.Duration
	ticker      time.Duration
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

func RouterWithTicker(ticker time.Duration) RouterOption {
	return func(c *RouterConfig) {
		c.ticker = ticker
	}
}

func RouterWithTimeout(timeout time.Duration) RouterOption {
	return func(c *RouterConfig) {
		c.timeout = &timeout
	}
}

func newRouter(opts ...RouterOption) *Router {
	config := &RouterConfig{
		initialSize: 1024,
		expandable:  true,
		throughput:  64,
		ticker:      time.Second / 5,
	}

	// defaults first
	r := &Router{
		mailbox: ringbuffer.New[message](
			ringbuffer.WithInitialSize(config.initialSize),
			ringbuffer.WithExpandable(config.expandable),
		),
		clock:    clock.New(clock.WithInterval(config.ticker)),
		registry: NewRuntimeRegistry(),
		shutdown: newSignal(),
		courier:  newCourier(),
	}

	for _, v := range opts {
		v(config)
	}

	r.ctx, r.ctxFn = context.WithCancel(context.Background())
	if config.timeout != nil {
		r.ctx, r.ctxFn = context.WithTimeout(r.ctx, *config.timeout)
	}

	// Router is responsible to start the registry
	r.clock.Add(r.registry, clock.ManagedTimeline)
	r.clock.Add(r.registry.mailbox, clock.ManagedTimeline)

	return r
}

func (r *Router) Add(name string, opts ...OptionRuntime) (uint, context.CancelFunc) {
	return r.registry.Add(name, opts...)
}

// Router is about dealing with intermediary messages between runtimes
// - poison to another runtime
func (r *Router) Tick() {
	fmt.Println("router tick")
	// var err error
	select {
	case msgs := <-r.mailbox.DataAvailable():
		// if len(msgs) == 0 {
		// 	fmt.Println("no messages")
		// 	continue
		// }

		fmt.Printf("router processing %v message(s)\n", len(msgs))
		forward := []message{}
		for i := 0; i < len(msgs); i++ {
			if value, ok := msgs[i].Metadata["$_gronosPoison"]; ok {
				fmt.Println("poison msg", value)
				continue
			}
			forward = append(forward, msgs[i])
		}

		fmt.Println("router forward", forward)

		for i := 0; i < len(forward); i++ {
			// if _, ok := forward[i].Metadata["$_gronos"]; ok {
			// 	var retries int
			// 	if retries, ok = forward[i].Metadata["$_gronos::retries"].(int); !ok {
			// 		slog.Error("error parsing retries", slog.Any("error", err))
			// 		// TODO add message identifier so we can deal with it later
			// 		r.courier.Transmit(errors.Join(ErrMessageMetadataParse, err)) // we don't care, someone else is going to manage it
			// 		continue
			// 	}
			// 	if retries < 5 {
			// 		retries++
			// 		forward[i].Metadata["$$_gronos::retries"] = fmt.Sprintf("%d", retries)
			// 		r.mailbox.Push(forward[i])
			// 		continue
			// 	}
			// }

			r.registry.Deliver(forward[i])
		}

	default:
		fmt.Println("no messages")
	}
}

func (r *Router) Tell(to uint, msg message) error {
	if to == 0 {
		return fmt.Errorf("missing receiver id")
	}
	msg.applyOptions(withGronos()) // our metadata
	msg.to = to                    // tag to
	return r.mailbox.Push(msg)
}

// // Complete close of the context of the runtimes, wait for shutdowns, then close the router
// func (r *Router) Close() {
// 	r.ctxFn() // trigger cancellation
// 	// r.wg.Wait()           // wait for runtimes
// 	r.mailbox.Close()     // close mailbox
// 	r.shutdown.Complete() // trigger shutdown
// }

// func (r *Router) ForceClose() {
// 	r.ctxFn()             // trigger cancellation
// 	r.mailbox.Close()     // close mailbox
// 	r.shutdown.Complete() // trigger shutdown
// }

// // Complete wait of both the runtimes and the router
// func (r *Router) Wait() {
// 	// r.wg.Wait()          // wait for runtimes
// 	<-r.shutdown.Await() // wait for router shutdown
// }

// // message processing
// func (r *Router) process() {
// 	for {
// 		select {

// 		case <-r.shutdown.Await():
// 			return

// 		// todo analyze the messages, trigger gronos things first, else dispatch
// 		// assume we already have `to` as uint
// 		case msgs := <-r.mailbox.DataAvailable():
// 			// if len(msgs) == 0 {
// 			// 	fmt.Println("no messages")
// 			// 	continue
// 			// }
// 			fmt.Printf("processing %v message(s)\n", len(msgs))
// 			forward := []envelope{}
// 			for i := 0; i < len(msgs); i++ {
// 				if msgs[i].Meta["$_gronosPoison"] != nil {
// 					fmt.Println("poison msg", msgs[i])
// 					continue
// 				}
// 				forward = append(forward, msgs[i])
// 			}
// 			fmt.Println("forward", forward)
// 			for i := 0; i < len(forward); i++ {
// 				if runtime, ok := r.runtimes.Get(forward[i].to); ok {
// 					fmt.Println("sending msg to ", runtime.name)
// 					runtime.courier.Deliver(forward[i])
// 				} else {
// 					// TODO monitor that error
// 					// TODO retry to send the message
// 					{
// 						counter := forward[i].Meta["$_gronosRetry"].(int)
// 						if counter < 5 {
// 							counter++
// 							forward[i].Meta["$_gronosRetry"] = counter
// 							r.mailbox.Push(forward[i])
// 						}
// 					}
// 					fmt.Println("receiver not found")
// 				}
// 			}
// 		}
// 	}
// }

// // health of runtimes
// func (r *Router) run() {
// 	for {
// 		select {
// 		case <-r.ctx.Done():
// 			r.ticker.Stop()
// 			r.ForceClose()
// 			fmt.Println("context is done")
// 			return

// 		case <-r.shutdown.Await():
// 			r.ticker.Stop()
// 			err := r.runtimes.ForEach(func(id uint, runtime *Runtime) error {
// 				runtime.signal.Complete()
// 				return nil
// 			})
// 			fmt.Println("error context done", err)
// 			return

// 		case <-r.ticker.C:
// 			// check the status of all runtimes
// 			if err := r.runtimes.ForEach(func(k uint, v *Runtime) error {
// 				slog.Info("checking runtime", slog.Any("id", k), slog.Any("name", v.name))
// 				var state runtimeState
// 				var err error
// 				var ok bool
// 				if state, ok = r.states.Get(k); !ok {
// 					return fmt.Errorf("runtime %d not found", k)
// 				}
// 				slog.Info("state runtime", slog.Any("id", k), slog.Any("name", v.name), slog.Any("state", state))
// 				// i don't think we should manage transition in states
// 				switch state {
// 				case added:
// 					slog.Info("starting runtime", slog.Any("id", k), slog.Any("name", v.name), slog.Any("state", state))
// 					r.states.Set(k, started)
// 					go r.runtime(k, v) // will change itself to failed or panicked
// 				case started:
// 				case stopped:
// 				case failed:
// 				case panicked:
// 				default:
// 					//
// 				}

// 				return err
// 			}); err != nil {
// 				fmt.Println("error foreach ", err)
// 			}
// 		}
// 	}
// }

// // `goroutine` when starting the runtime
// func (r *Router) runtime(id uint, run *Runtime) {
// 	// it's already a goroutine, we just manage the other goroutines from here
// 	// goroutine for the runtime, we will have to manage it here
// 	go func() {
// 		defer func() {
// 			if re := recover(); re != nil {
// 				r.states.Set(id, panicked)
// 				fmt.Println("recovering packing", re)
// 			}
// 		}()
// 		r.wg.Add(1)
// 		slog.Info("runtime running", slog.Any("id", id), slog.Any("name", run.name))
// 		if err := run.runtime(run.ctx, run.mailbox, run.courier, run.signal); err != nil {
// 			slog.Error("Gronos runtime error", slog.Any("id", run.id), slog.Any("error", err))
// 		}
// 		slog.Info("runtime done", slog.Any("id", id), slog.Any("name", run.name))
// 		r.wg.Done()
// 	}()
// }

// func (r *Router) WhenID(id uint, state runtimeState) <-chan struct{} {
// 	waitc := make(chan struct{})

// 	go func() {
// 		ticker := time.NewTicker(time.Millisecond * 50)
// 		for range ticker.C {
// 			if s, ok := r.states.Get(id); ok && s == state {
// 				close(waitc)
// 				return
// 			}
// 		}
// 	}()

// 	return waitc
// }

// func (c *Router) getID(name string) uint {
// 	if id, ok := c.runtimesNames.Get(name); ok {
// 		return *id
// 	}
// 	return 0
// }

// func (c *Router) getName(id uint) string {
// 	if data, ok := c.runtimes.Get(id); ok {
// 		return data.name
// 	}
// 	return ""
// }

// func (r *Router) remove(id uint) error {
// 	name := r.getName(id)
// 	if name != "" {
// 		return fmt.Errorf("runtime %d not found", id)
// 	}
// 	r.runtimes.Delete(id)
// 	r.runtimesNames.Delete(name)
// 	r.states.Delete(id)
// 	return nil
// }

// func (r *Router) Direct(msg Message, to uint) error {
// 	if _, ok := r.runtimes.Get(to); ok {
// 		// runtime.courier.Deliver(envelope{to: to, Msg: msg})

// 		e := envelope{to: to, Msg: msg, Meta: map[string]interface{}{}}
// 		e.Meta["$_gronosRetry"] = 0
// 		return r.mailbox.Push(e) // it will be post-process
// 	}
// 	return fmt.Errorf("receiver not found")
// }

// // `Delivery` require the name of the runtime, more readable but slower
// func (c *Router) named(msg Message, name string) error {
// 	if id, ok := c.runtimesNames.Get(name); ok {
// 		// if runtime, okID := c.runtimes.Get(*id); okID {
// 		// 	fmt.Println("courrier delivery envelope to ", runtime.name)
// 		// 	// runtime.courier.Deliver(envelope{to: *id, Msg: msg})
// 		// 	return nil
// 		// }
// 		e := envelope{to: *id, Msg: msg, Meta: map[string]interface{}{}}
// 		e.Meta["$_gronosRetry"] = 0
// 		return c.mailbox.Push(e) // it will be post-process
// 	}
// 	return fmt.Errorf("receiver not found")
// }

// func (c *Router) broadcast(msg Message) error {
// 	return c.runtimes.ForEach(func(id uint, runtime *Runtime) error {
// 		runtime.courier.Deliver(envelope{to: id, Msg: msg})
// 		return nil
// 	})
// }

// func (c *Router) transmit(err error, to uint) error {
// 	return c.runtimes.ForEach(func(_id uint, runtime *Runtime) error {
// 		if _id == to {
// 			runtime.courier.Transmit(err)
// 		}
// 		return nil
// 	})
// }

// // cancel will stop the runtime immediately, your runtime will eventually trigger it's own shutdown
// func (c *Router) cancel(id uint) error {
// 	return c.runtimes.ForEach(func(_id uint, runtime *Runtime) error {
// 		if _id == id {
// 			runtime.cancel()
// 		}
// 		return nil
// 	})
// }

// func (c *Router) cancelAll() error {
// 	return c.runtimes.ForEach(func(id uint, runtime *Runtime) error {
// 		runtime.cancel()
// 		return nil
// 	})
// }

// func (c *Router) directPause(id uint) error {
// 	return c.runtimes.ForEach(func(_id uint, runtime *Runtime) error {
// 		if _id == id {
// 			pause, _, ok := PlayPauseOperations(runtime.ctx)
// 			if ok {
// 				pause()
// 			}
// 		}
// 		return nil
// 	})
// }

// func (c *Router) directResume(id uint) error {
// 	return c.runtimes.ForEach(func(_id uint, runtime *Runtime) error {
// 		if _id == id {
// 			_, resume, ok := PlayPauseOperations(runtime.ctx)
// 			if ok {
// 				resume()
// 			}
// 		}
// 		return nil
// 	})
// }

// func (c *Router) directComplete(id uint) error {
// 	return c.runtimes.ForEach(func(_id uint, runtime *Runtime) error {
// 		if _id == id {
// 			runtime.signal.Complete()
// 		}
// 		return nil
// 	})
// }

// func (c *Router) directPhasingOut(id uint) error {
// 	if runtime, ok := c.runtimes.Get(id); ok {
// 		pause, _, ok := PlayPauseOperations(runtime.ctx)
// 		if ok {
// 			pause()
// 		}
// 		runtime.signal.Complete()
// 		<-runtime.signal.Await()
// 		return nil
// 	}
// 	return fmt.Errorf("runtime not found")
// }
