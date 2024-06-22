package gronos

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/davidroman0O/gronos/ringbuffer"
)

type RegistryState uint

const (
	stateAdded RegistryState = iota
	stateStarted
	stateStopped
	stateFailed
	statePanicked
)

// Control the state of the runtime functions
type RuntimeRegistry struct {
	ctx    context.Context
	cancel context.CancelFunc

	flipID        *flipID
	runtimesNames *safeMapPtr[string, uint]
	runtimes      *safeMapPtr[uint, Runtime]
	states        *safeMap[uint, RegistryState]

	mailbox *ringbuffer.RingBuffer[message]

	shutdown *Signal
	wg       sync.WaitGroup
}

func NewRuntimeRegistry() *RuntimeRegistry {
	registry := &RuntimeRegistry{
		flipID:        newFlipID(), // helper to assign ID without collision
		runtimesNames: newSafeMapPtr[string, uint](),
		runtimes:      newSafeMapPtr[uint, Runtime](),
		states:        newSafeMap[uint, RegistryState](),
		mailbox: ringbuffer.New[message](
			ringbuffer.WithInitialSize(300),
			ringbuffer.WithExpandable(true),
		), // TODO make parameters
		shutdown: newSignal(),
	}

	registry.ctx, registry.cancel = context.WithCancel(context.Background())

	return registry
}

func (r *RuntimeRegistry) WhenID(id uint, state RegistryState) <-chan struct{} {
	waitc := make(chan struct{})

	go func() {
		ticker := time.NewTicker(time.Millisecond * 50)
		for range ticker.C {
			if s, ok := r.states.Get(id); ok && s == state {
				close(waitc)
				return
			}
		}
	}()

	return waitc
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
			shutdown, _ := UseShutdown(v.ctx)
			<-shutdown.Await()
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
		fmt.Printf("registry processing %v message(s)\n", len(msgs))
		forward := []message{}
		for i := 0; i < len(msgs); i++ {
			if value, ok := msgs[i].Metadata["$_gronosPoison"]; ok {
				fmt.Println("poison msg", value)
				continue
			}
			forward = append(forward, msgs[i])
		}
		fmt.Println("registry forward", forward)
		for i := 0; i < len(forward); i++ {
			if runtime, ok := r.runtimes.Get(forward[i].to); ok {
				fmt.Println("sending msg to ", runtime.name)
				courier, _ := UseCourier(runtime.ctx)
				courier.DeliverMany(forward)
			} else {
				// TODO monitor that error
				// TODO retry to send the message
				fmt.Println("receiver not found")
			}
		}

	default:
		slog.Info("Registry nothing tick")
	}

	// TODO should no be there, should be event driven OR maybe it's ok
	// check the status of all runtimes
	if err := r.runtimes.ForEach(func(k uint, v *Runtime) error {
		var state RegistryState
		var err error
		var ok bool
		if state, ok = r.states.Get(k); !ok {
			return fmt.Errorf("runtime %d not found", k)
		}
		// i don't think we should manage transition in states
		switch state {
		case stateAdded:
			slog.Info("starting runtime", slog.Any("id", k), slog.Any("name", v.name), slog.Any("state", state))
			<-r.Start(k, v) // will change itself to failed or panicked
		default:
			// v.mailbox.buffer.Tick() // TODO we need a clock management for all
		}
		return err
	}); err != nil {
		fmt.Println("error foreach ", err)
	}
}

func (e *RuntimeRegistry) Get(id uint) (*Runtime, bool) {
	return e.runtimes.Get(id)
}

func (e *RuntimeRegistry) Add(name string, opts ...OptionRuntime) (uint, context.CancelFunc) {
	id := e.flipID.Next()

	opts = append(opts, RuntimeWithID(id))
	runtime := newRuntime(name, opts...)

	// In this order so no system can access it until the last map
	e.runtimesNames.Set(runtime.name, &runtime.id)
	e.states.Set(runtime.id, stateAdded)
	e.runtimes.Set(runtime.id, runtime)

	return runtime.id, runtime.cancel
}

func (r *RuntimeRegistry) Deliver(msg message) {
	// if runtime, ok := r.runtimes.Get(msg.to); ok {
	// 	runtime.courier.Deliver(msg)
	// }
	if _, ok := r.runtimes.Get(msg.to); ok {
		slog.Info("Registry Delivery", slog.String("system", "runtime registry"), slog.Any("to", msg.to))
		r.mailbox.Push(msg)
	}
}

func (r *RuntimeRegistry) DeliverMany(to uint, msgs []message) {
	// if runtime, ok := r.runtimes.Get(to); ok {
	// 	runtime.courier.DeliverMany(msgs)
	// }
	if _, ok := r.runtimes.Get(to); ok {
		for _, v := range msgs {
			slog.Info("Registry Delivery", slog.String("system", "runtime registry"), slog.Any("to", to))
			r.mailbox.Push(v)
		}
	}
}

// Start a runtime, to be used on a goroutine
func (r *RuntimeRegistry) Start(id uint, run *Runtime) <-chan struct{} {
	// runtimes api is already a fsm, there is not need to build one
	// TODO should have a context to be sure it started
	return run.Start(
		WithBeforeStart(func() {
			slog.Info("before start")
			r.wg.Add(1)
			r.states.Set(id, stateStarted)
		}),
		WithPanic(func(recover interface{}) {
			slog.Info("panic: ", slog.Any("recover", recover))
			r.wg.Done()
			r.states.Set(id, statePanicked)
		}),
		WithAfterStop(func() {
			slog.Info("after stop")
			r.wg.Done()
			r.states.Set(id, stateStopped)
		}),
		WithFailed(func(err error) {
			slog.Info("failed: ", slog.Any("error", err))
			r.wg.Done()
			r.states.Set(id, stateFailed)
		}),
	).Await()
}
