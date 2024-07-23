package gronos

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/davidroman0O/gronos/clock"
	"github.com/davidroman0O/gronos/ringbuffer"
)

/// This file is an attempt to rewrite everything in ONE file (not really since ringbuffer and clocks are isolated) to fit as much of what i've learned into a real shit

type flipID struct {
	current uint
}

func newFlipID() *flipID {
	return &flipID{
		current: 0, // 0 means no id, we will use Next() all the time which mean we will have `1` always as a starter
	}
}

func (f *flipID) Next() uint {
	f.current++
	return f.current
}

var metadataTo = "to"
var metadataReplyAddress = "replyAddress"
var metadataReturnAddress = "returnAddress"

// at runtime, interface{} is cool
type Metadata map[string]interface{}

// actual data given by user
type Payload interface{}

// TODO: be interfaceable with `watermill` cause i love those guys
type Message struct {
	// messages will be exchanged mostly at runtime
	// could be useful to have a context to add timeouts, deadlines, etc
	context.Context
	metadata Metadata
	payload  Payload
}

func NewMessage(ctx context.Context, metadata Metadata, payload Payload) Message {
	return Message{
		Context:  ctx,
		metadata: metadata,
		payload:  payload,
	}
}

type metadataOption func(*Metadata)

func WithTo(to string) metadataOption {
	return func(m *Metadata) {
		(*m)[metadataTo] = to
	}
}

func WithReplyTo(replyTo string) metadataOption {
	return func(m *Metadata) {
		(*m)[metadataReplyAddress] = replyTo
	}
}

func WithReturnTo(returnTo string) metadataOption {
	return func(m *Metadata) {
		(*m)[metadataReturnAddress] = returnTo
	}
}

func NewMetadata(opts ...metadataOption) Metadata {
	m := Metadata{}
	for _, opt := range opts {
		opt(&m)
	}
	return m
}

type Signal struct {
	closed bool
	c      chan struct{}
}

func (s *Signal) Complete() {
	if s.closed {
		return
	}
	close(s.c)
	s.closed = true
}

func (s *Signal) Await() <-chan struct{} {
	return s.c
}

func newSignal() *Signal {
	c := make(chan struct{})
	return &Signal{
		c: c,
	}
}

func doMessages(rb *ringbuffer.RingBuffer[Message], cb func(msgs []Message) error) chan error {
	done := make(chan error)
	go func() {
		defer close(done)
		if rb.HasData() {
			var msg []Message
			msg, open := <-rb.DataAvailable()
			if open {
				if err := cb(msg); err != nil {
					done <- err
				}
			}
			// TODO: what do we do when close?
		}
	}()
	return done
}

func FnCourierPush(rb *ringbuffer.RingBuffer[Message]) func(msg Message) error {
	return func(msg Message) error {
		return rb.Push(msg)
	}
}

type RegistryState uint

const (
	stateAdded RegistryState = iota
	stateStarted
	stateStopped
	stateFailed
	statePanicked
)

// One gronos per runtime, as single as that
// Maybe one day i will implement a namespace thing so we could have multiple gronos in the same runtime
type Gronos struct {
	*clock.Clock
	courier      *ringbuffer.RingBuffer[Message]
	router       *ringbuffer.RingBuffer[Message]
	courierClock *clock.Clock
	routerClock  *clock.Clock

	flipID        *flipID
	runtimesNames *safeMapPtr[string, uint]
	runtimes      *safeMapPtr[uint, Runtime]
	states        *safeMap[uint, RegistryState]
	office        *safeMap[uint, *ringbuffer.RingBuffer[Message]]
	watchmaker    *safeMap[uint, *clock.Clock]

	wg sync.WaitGroup
}

type Config struct {
	routerInterval  time.Duration
	courierInterval time.Duration
}

type OptionConfig func(*Config)

func WithRouterInterval(d time.Duration) OptionConfig {
	return func(c *Config) {
		c.routerInterval = d
	}
}

func WithCourierInterval(d time.Duration) OptionConfig {
	return func(c *Config) {
		c.courierInterval = d
	}
}

func New(opts ...OptionConfig) *Gronos {
	c := Config{
		routerInterval:  250 * time.Millisecond,
		courierInterval: 250 * time.Millisecond,
	}
	for _, opt := range opts {
		opt(&c)
	}
	g := &Gronos{
		Clock: clock.New(
			clock.WithName("gronos"),
			clock.WithInterval(10*time.Millisecond)),
		routerClock: clock.New(
			clock.WithName("router"),
			clock.WithInterval(c.routerInterval)),
		router: ringbuffer.New[Message](
			ringbuffer.WithName("router"),
			ringbuffer.WithInitialSize(1024*8),
			ringbuffer.WithExpandable(true),
		),
		courierClock: clock.New(
			clock.WithName("courier"),
			clock.WithInterval(c.courierInterval)),
		courier: ringbuffer.New[Message](
			ringbuffer.WithName("courier"),
			ringbuffer.WithInitialSize(1024*8),
			ringbuffer.WithExpandable(true),
		),
		flipID:        newFlipID(), // helper to assign ID without collision
		runtimesNames: newSafeMapPtr[string, uint](),
		runtimes:      newSafeMapPtr[uint, Runtime](),
		states:        newSafeMap[uint, RegistryState](),
		office:        newSafeMap[uint, *ringbuffer.RingBuffer[Message]](),
		watchmaker:    newSafeMap[uint, *clock.Clock](),
	}
	g.Clock.Add(g, clock.BestEffort)
	g.Clock.Start()
	g.routerClock.Add(g.router, clock.ManagedTimeline)
	g.courierClock.Add(g.courier, clock.ManagedTimeline)
	g.routerClock.Start()
	g.courierClock.Start()
	return g
}

func (g *Gronos) Close() {
	g.routerClock.Stop()
	g.courierClock.Stop()
	// TODO check for datarace here
	for _, v := range g.runtimes.m {
		s, ok := UseShutdown(v.ctx)
		if ok {
			s.Complete()
		}
	}
	g.wg.Wait()
}

func (g *Gronos) Push(msg Message) error {
	return g.router.Push(msg)
}

func (g *Gronos) Add(name string, opts ...OptionRuntime) (uint, context.CancelFunc) {
	id := g.flipID.Next()

	opts = append(opts, RuntimeWithID(id))

	mailbox := ringbuffer.New[Message](
		ringbuffer.WithName(name),
		ringbuffer.WithInitialSize(1024*8),
		ringbuffer.WithExpandable(true),
	)
	g.office.Set(id, mailbox)

	c := clock.New(
		clock.WithName(name),
		clock.WithInterval(10*time.Millisecond),
	)
	c.Add(mailbox, clock.ManagedTimeline)
	c.Start()
	g.watchmaker.Set(id, c)

	runtime := newRuntime(name, mailbox, g.courier, c, opts...)

	// In this order so no system can access it until the last map
	g.runtimesNames.Set(runtime.name, &runtime.id)
	g.states.Set(runtime.id, stateAdded)
	g.runtimes.Set(runtime.id, runtime)

	return runtime.id, runtime.cancel
}

func (g *Gronos) Tick() {

	// fmt.Println("tick")
	// fmt.Println("router has data", g.router.HasData())
	// fmt.Println("courier has data", g.courier.HasData())

	// router first
	whenRouterMsgProcessed := doMessages(
		g.router,
		// TODO: each function should be specialized
		func(msgs []Message) error {
			dispatchMap := map[string][]Message{}
			for _, msg := range msgs {
				if to, ok := msg.metadata[metadataTo].(string); ok {
					if _, ok := dispatchMap[to]; !ok {
						dispatchMap[to] = []Message{}
					}
					dispatchMap[to] = append(dispatchMap[to], msg)
				}
				// fmt.Println("router", msg)
			}
			fmt.Println("dispatchMap", len(dispatchMap))
			for to, msgs := range dispatchMap {
				if id, ok := g.runtimesNames.Get(to); ok {
					// go func(runtimeID uint) {
					if mailbox, ok := g.office.Get(*id); ok {
						for i := 0; i < len(msgs); i++ {
							mailbox.Push(msgs[i])
						}
					}
					// }(*id)
				}
			}
			return nil
		})

	// courier will wait for router to be processed
	whenCourierMsgProcessed := doMessages(
		g.courier,
		// TODO: each function should be specialized
		func(msgs []Message) error {
			<-whenRouterMsgProcessed
			for _, msg := range msgs {
				// fmt.Println("courier", msg)
				g.router.Push(msg)
			}
			return nil
		})

	<-whenCourierMsgProcessed // when courier is done, we can move on

	// fmt.Println("processing runtimes")
	if err := g.runtimes.ForEach(func(k uint, v *Runtime) error {
		var state RegistryState
		var err error
		var ok bool
		if state, ok = g.states.Get(k); !ok {
			return fmt.Errorf("runtime %d not found", k)
		}
		// i don't think we should manage transition in states
		switch state {
		case stateAdded:
			// slog.Info("starting runtime", slog.Any("id", k), slog.Any("name", v.name), slog.Any("state", state))
			<-g.Start(k, v) // will change itself to failed or panicked
		default:
			// v.mailbox.buffeg.Tick() // TODO we need a clock management for all
		}
		return err
	}); err != nil {
		fmt.Println("error foreach ", err)
	}
}

// Start a runtime, to be used on a goroutine
func (r *Gronos) Start(id uint, run *Runtime) <-chan struct{} {
	// runtimes api is already a fsm, there is not need to build one
	// TODO should have a context to be sure it started
	return run.Start(
		WithBeforeStart(func() {
			// slog.Info("before start")
			r.wg.Add(1)
			r.states.Set(id, stateStarted)
		}),
		WithPanic(func(recover interface{}) {
			// slog.Info("panic: ", slog.Any("recover", recover))
			r.wg.Done()
			r.states.Set(id, statePanicked)
		}),
		WithAfterStop(func() {
			// slog.Info("after stop")
			r.wg.Done()
			r.states.Set(id, stateStopped)
		}),
		WithFailed(func(err error) {
			// slog.Info("failed: ", slog.Any("error", err))
			r.wg.Done()
			r.states.Set(id, stateFailed)
		}),
	).Await()
}
