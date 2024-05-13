package gronos

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
)

/// TODO Task based ticker to modify the state of runtimes
/// TODO state should be handled ONCE per transition, that mean i need a transition controller somehow or my own fsm
/// Each runtime will have their own fsm, each fsm will be plugged into the callbacks of the runtime to transitions all states

type RegistryState uint

const (
	added RegistryState = iota
	started
	stopped
	failed
	panicked
)

// Control the state of the runtime functions
type RuntimeRegistry struct {
	ctx    context.Context
	cancel context.CancelFunc

	flipID        *FlipID
	runtimesNames *safeMapPtr[string, uint]
	runtimes      *safeMapPtr[uint, Runtime]
	states        *safeMap[uint, RegistryState]

	mailbox *RingBuffer[envelope]

	shutdown *Signal
	wg       sync.WaitGroup
}

func NewRuntimeRegistry() *RuntimeRegistry {
	registry := &RuntimeRegistry{
		flipID:        NewFlipID(), // helper to assign ID without collision
		runtimesNames: newSafeMapPtr[string, uint](),
		runtimes:      newSafeMapPtr[uint, Runtime](),
		states:        newSafeMap[uint, RegistryState](),
		mailbox:       NewRingBuffer[envelope](), // TODO make parameters
		shutdown:      newSignal(),
	}

	registry.ctx, registry.cancel = context.WithCancel(context.Background())

	return registry
}

func (r *RuntimeRegistry) Tick() {
	select {

	case <-r.ctx.Done():
		slog.Info("shutting down runtime registry")
		r.shutdown.Complete()

	case <-r.shutdown.Await():
		if err := r.runtimes.ForEach(func(k uint, v *Runtime) error {
			slog.Info("cancelling runtime", slog.Any("id", k), slog.Any("name", v.name))
			v.cancel()
			slog.Info("waiting runtime", slog.Any("id", k), slog.Any("name", v.name))
			<-v.signal.Await()
			return nil
		}); err != nil {
			fmt.Println("error foreach ", err)
		}
		return

	// todo analyze the messages, trigger gronos things first, else dispatch
	// assume we already have `to` as uint
	case msgs := <-r.mailbox.DataAvailable():
		// if len(msgs) == 0 {
		// 	fmt.Println("no messages")
		// 	continue
		// }
		fmt.Printf("processing %v message(s)\n", len(msgs))
		forward := []envelope{}
		for i := 0; i < len(msgs); i++ {
			if msgs[i].Meta["$_gronosPoison"] != nil {
				fmt.Println("poison msg", msgs[i])
				continue
			}
			forward = append(forward, msgs[i])
		}
		fmt.Println("forward", forward)
		for i := 0; i < len(forward); i++ {
			if runtime, ok := r.runtimes.Get(forward[i].to); ok {
				fmt.Println("sending msg to ", runtime.name)
				runtime.courier.Deliver(forward[i])
			} else {
				// TODO monitor that error
				// TODO retry to send the message
				{
					counter := forward[i].Meta["$_gronosRetry"].(int)
					if counter < 5 {
						counter++
						forward[i].Meta["$_gronosRetry"] = counter
						r.mailbox.Push(forward[i])
					}
				}
				fmt.Println("receiver not found")
			}
		}

	default:
		fmt.Println("runtime nothing")
	}

	// check the status of all runtimes
	if err := r.runtimes.ForEach(func(k uint, v *Runtime) error {
		slog.Info("checking runtime", slog.Any("id", k), slog.Any("name", v.name))
		var state RegistryState
		var err error
		var ok bool
		if state, ok = r.states.Get(k); !ok {
			return fmt.Errorf("runtime %d not found", k)
		}
		slog.Info("state runtime", slog.Any("id", k), slog.Any("name", v.name), slog.Any("state", state))
		// i don't think we should manage transition in states
		switch state {
		case added:
			slog.Info("starting runtime", slog.Any("id", k), slog.Any("name", v.name), slog.Any("state", state))
			<-r.Start(k, v) // will change itself to failed or panicked
		case started:
		case stopped:
		case failed:
		case panicked:
		default:
			//
		}

		return err
	}); err != nil {
		fmt.Println("error foreach ", err)
	}
}

func (e *RuntimeRegistry) Add(name string, opts ...OptionRuntime) (uint, context.CancelFunc) {
	id := e.flipID.Next()

	opts = append(opts, RuntimeWithID(id))
	runtime := newRuntime(name, opts...)

	// In this order so no system can access it until the last map
	e.runtimesNames.Set(runtime.name, &runtime.id)
	e.states.Set(runtime.id, added)
	e.runtimes.Set(runtime.id, runtime)

	return runtime.id, runtime.cancel
}

func (r *RuntimeRegistry) Deliver(msg envelope) {
	if runtime, ok := r.runtimes.Get(msg.to); ok {
		slog.Info("Delivery", slog.String("system", "runtime registry"), slog.String("to", msg.name))
		runtime.courier.Deliver(msg)
	}
}

func (r *RuntimeRegistry) DeliverMany(to uint, msgs []envelope) {
	if runtime, ok := r.runtimes.Get(to); ok {
		slog.Info("Delivery", slog.String("system", "runtime registry"), slog.Any("to", to))
		runtime.courier.DeliverMany(msgs)
	}
}

// Start a runtime, to be used on a goroutine
func (r *RuntimeRegistry) Start(id uint, run *Runtime) <-chan struct{} {
	return run.Start(
		WithAfterStart(func() {
			slog.Info("after start")
			r.wg.Add(1)
			r.states.Set(id, started)
		}),
		WithPanic(func(recover interface{}) {
			slog.Info("panic: ", slog.Any("recover", recover))
			r.wg.Done()
			r.states.Set(id, panicked)
		}),
		WithAfterStop(func() {
			slog.Info("after stop")
			r.wg.Done()
			r.states.Set(id, stopped)
		}),
	).Await()
}
