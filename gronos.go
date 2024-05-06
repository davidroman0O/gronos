package gronos

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"
)

/// Gronos organize the lifecycle of runtimes to help you compose your applications.
/// See runtimes and channels as gears, we're building a digital machine

/// Features
/// - basic runtimes functions
/// - play/pause context
/// - communication between runtimes
/// -

// TODO Replace API with ID and use string for names
// allowing both ID and name
// TODO runtimes should send status: ready, paused, stopped
// TODO ONE mainly Circular buffer for all message analysis, when publishing a message then analyze them in batches and then eventually push them back to other runtimes
// TODO we should send messages like `Send("namespace.name", msg)`
// TODO gronos should check if the namespace is its own, otherwise try to send it to a middleman
// TODO gronos to gronos with namespaces
// TODO increase i/o with circular buffers and configurable channel sizes (how to manage full buffer?)
// TODO make a good documentation

// TODO: use watermill's message router to send messages to other gronos instances
// Eventually, we could configure `gronos` with external pubsub if necessary

/// My general idea is to be able to split the application into smaller parts that can be managed independently.
/// I want to compose my application in a DDD way (with my opinions on it) while not conflicting domains.

/// I think that a runtime could be splitted into multiple runtime and even on different machines.
/// Eventually, you might want to have your application running different workloads for different types of processing on different hardware.
/// Exchanging messages within a runtime or between applications of the same fleet should be easy to do and configure.
/// Ideally, it would be up to the person that want to deploy the app to decide how to split the workloads and how to communicate between them with which third-party (sql, redis or rabbitmq).

type playPauseContext struct {
	context.Context
	done   chan struct{}
	pause  chan struct{}
	resume chan struct{}
}

type Pause <-chan struct{}
type Play <-chan struct{}

type gronosKey string

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

type FlipID struct {
	current uint
}

func NewFlipID() *FlipID {
	return &FlipID{
		current: 1, // 0 means no id
	}
}

func (f *FlipID) Next() uint {
	f.current++
	return f.current
}

type Shutdown uint

const (
	Gracefull Shutdown = iota
	Immediate
)

type Message interface{}

type envelope struct {
	to   uint
	name string
	Msg  Message
}

func Envelope(name string, msg Message) envelope {
	return envelope{
		to:   0,
		name: name,
		Msg:  msg,
	}
}

// It's a Mailbox
type Mailbox struct {
	closed bool
	r      chan envelope
	// TODO: add optional circular buffer
}

func (s *Mailbox) Read() <-chan envelope {
	return s.r
}

func (r *Mailbox) post(msg envelope) {
	r.r <- msg
}

// todo check for unecessary close
func (s *Mailbox) Complete() {
	if s.closed {
		return
	}
	close(s.r)
	s.closed = true
}

// TODO make the size of those channels configurable
func newMailbox() *Mailbox {
	return &Mailbox{
		r: make(chan envelope),
	}
}

// Courier is responsible for delivering error messages. When an error occurs, it's like the Courier "picks up" the error and "delivers" to the central mail station
type Courier struct {
	closed bool
	c      chan error
	e      chan envelope

	// TODO: add optional circular buffer
}

func (s *Courier) readNotices() <-chan error {
	return s.c
}

func (s *Courier) readMails() <-chan envelope {
	return s.e
}

func (s *Courier) Deliver(env envelope) {
	if s.closed {
		// slog.Info("Courier closed")
		return
	}
	if env.Msg != nil {
		s.e <- env
	}
}

func (s *Courier) Transmit(err error) {
	if s.closed {
		// slog.Info("Courier closed")
		return
	}
	if err != nil {
		s.c <- err
	}
}

// todo check for unecessary close
func (s *Courier) Complete() {
	if s.closed {
		return
	}
	s.closed = true
	close(s.c)
	close(s.e)
}

func newCourier() *Courier {
	c := make(chan error, 1)    // should be 3 modes: unbuffered, buffered, with 1 buffered channel to prevent panic on multiple ctrl+c signals
	e := make(chan envelope, 1) // should be 3 modes: unbuffered, buffered, with 1 buffered channel to prevent panic on multiple ctrl+c signals
	return &Courier{
		closed: false,
		c:      c,
		e:      e,
	}
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

// RuntimeFunc represents a function that runs a runtime.
type RuntimeFunc func(ctx context.Context, mailbox *Mailbox, courrier *Courier, shutdown *Signal) error

// Centralized place that manage the lifecycle of runtimes
type Gronos struct {
	shutdownMode Shutdown
	// runtimesNames map[string]uint
	runtimesNames *safeMap[string, uint]
	// runtimes      map[uint]RuntimeStation
	runtimes  *safeMap[uint, RuntimeStation]
	shutdown  *Signal
	wg        sync.WaitGroup
	finished  bool
	running   uint
	flipID    *FlipID
	courier   *Courier
	logger    Logger
	toggleLog bool

	mailbox *Mailbox // gronos to gronos
}

type Option func(*Gronos) error

// Interuption won't wait for runtimes to gracefully finish
func WithImmediateShutdown() Option {
	return func(c *Gronos) error {
		c.shutdownMode = Immediate
		return nil
	}
}

func WithLogger(logger Logger) Option {
	return func(c *Gronos) error {
		c.logger = logger
		return nil
	}
}

func WithSlogLogger() Option {
	return func(c *Gronos) error {
		c.logger = NewSlog()
		return nil
	}
}

func WithFmtLogger() Option {
	return func(c *Gronos) error {
		c.logger = NewFmt()
		return nil
	}
}

// Interuption will wait for runtimes to gracefully finish
func WithGracefullShutdown() Option {
	return func(c *Gronos) error {
		c.shutdownMode = Gracefull
		return nil
	}
}

// One runtime can receive and send messages while performing it's own task
type RuntimeStation struct {
	name    string // easier to reason with
	id      uint
	ctx     context.Context
	runtime RuntimeFunc
	cancel  context.CancelFunc
	courier *Courier
	mailbox *Mailbox
	signal  *Signal
}

// not sure if i'm going to use the error here
type OptionRuntime func(*RuntimeStation) error

func WithContext(ctx context.Context) OptionRuntime {
	return func(r *RuntimeStation) error {
		r.ctx = ctx
		return nil
	}
}

func WithTimeout(d time.Duration) OptionRuntime {
	return func(r *RuntimeStation) error {
		ctx, cnfn := context.WithTimeout(r.ctx, d)
		r.cancel = cnfn
		r.ctx = ctx
		return nil
	}
}

func WithValue(key, value interface{}) OptionRuntime {
	return func(r *RuntimeStation) error {
		r.ctx = context.WithValue(r.ctx, key, value)
		return nil
	}
}

// func WithCancel() OptionRuntime {
// 	return func(r *RuntimeStation) error {
// 		ctx, cnfn := context.WithCancel(r.ctx)
// 		r.cancel = cnfn
// 		r.ctx = ctx
// 		return nil
// 	}
// }

func WithDeadline(d time.Time) OptionRuntime {
	return func(r *RuntimeStation) error {
		ctx, cfn := context.WithDeadline(r.ctx, d)
		r.cancel = cfn
		r.ctx = ctx
		return nil
	}
}

func WithRuntime(r RuntimeFunc) OptionRuntime {
	return func(rc *RuntimeStation) error {
		rc.runtime = r
		return nil
	}
}

func (c *Gronos) Add(name string, opts ...OptionRuntime) (uint, context.CancelFunc) {
	id := c.flipID.Next()
	r := RuntimeStation{
		name:    name,
		id:      id,
		courier: newCourier(),
		mailbox: newMailbox(),
		signal:  newSignal(),
	}
	ctx := context.Background()
	ctx = context.WithValue(ctx, getIDKey, c.getID)
	r.ctx, r.cancel = WithPlayPause(ctx)
	for _, opt := range opts {
		opt(&r)
	}
	c.runtimes.Set(id, &r)
	c.runtimesNames.Set(name, &id)
	return id, r.cancel
}

// func (c *Gronos) AddFuture() uint {
// 	return c.flipID.Next()
// }

// func (c *Gronos) Push(name string, opts ...OptionRuntime) (uint, context.CancelFunc) {
// 	id := c.flipID.Next()
// 	r := RuntimeStation{
// 		id:      id,
// 		courier: newCourier(),
// 		mailbox: newMailbox(),
// 		signal:  newSignal(),
// 	}
// 	ctx := context.Background()
// 	r.ctx, r.cancel = WithPlayPause(ctx)
// 	for _, opt := range opts {
// 		opt(&r)
// 	}
// 	c.runtimes[id] = r
// 	return id, r.cancel
// }

// `Direct` require the ID of the runtime, faster but less readable
func (c *Gronos) Direct(msg Message, to uint) error {
	if runtime, ok := c.runtimes.Get(to); ok {
		runtime.courier.Deliver(envelope{to: to, Msg: msg})
	}
	return fmt.Errorf("receiver not found")
}

// `Delivery` require the name of the runtime, more readable but slower
func (c *Gronos) Named(msg Message, name string) error {
	if id, ok := c.runtimesNames.Get(name); ok {
		if runtime, okID := c.runtimes.Get(*id); okID {
			runtime.courier.Deliver(envelope{to: *id, Msg: msg})
		}
	}
	return fmt.Errorf("receiver not found")
}

func (c *Gronos) Broadcast(msg Message) {
	c.runtimes.ForEach(func(id uint, runtime *RuntimeStation) {
		runtime.courier.Deliver(envelope{to: id, Msg: msg})
	})
}

func (c *Gronos) Transmit(err error, to uint) {
	c.runtimes.ForEach(func(id uint, runtime *RuntimeStation) {
		runtime.courier.Transmit(err)
	})
}

// Cancel will stop the runtime immediately, your runtime will eventually trigger it's own shutdown
func (c *Gronos) Cancel(id uint) {
	c.runtimes.ForEach(func(id uint, runtime *RuntimeStation) {
		runtime.cancel()
	})
}

func (c *Gronos) CancelAll() {
	c.runtimes.ForEach(func(id uint, runtime *RuntimeStation) {
		runtime.cancel()
	})
}

func (c *Gronos) DirectPause(id uint) {
	c.runtimes.ForEach(func(id uint, runtime *RuntimeStation) {
		pause, _, ok := PlayPauseOperations(runtime.ctx)
		if ok {
			pause()
		}
	})
}

func (c *Gronos) DirectResume(id uint) {
	c.runtimes.ForEach(func(id uint, runtime *RuntimeStation) {
		_, resume, ok := PlayPauseOperations(runtime.ctx)
		if ok {
			resume()
		}
	})
}

func (c *Gronos) DirectComplete(id uint) {
	c.runtimes.ForEach(func(id uint, runtime *RuntimeStation) {
		runtime.signal.Complete()
	})
}

func (c *Gronos) DirectPhasingOut(id uint) {
	if runtime, ok := c.runtimes.Get(id); ok {
		pause, _, ok := PlayPauseOperations(runtime.ctx)
		if ok {
			pause()
		}
		runtime.signal.Complete()
		<-runtime.signal.Await()
	}
}

func New(opts ...Option) (*Gronos, error) {
	ctx := &Gronos{
		runtimes:      newSafeMap[uint, RuntimeStation](), // faster usage - dev could use only that to be faster
		runtimesNames: newSafeMap[string, uint](),         // convienient usage - dev could use that to be more readable

		shutdown:     newSignal(),
		finished:     false,
		running:      0,
		shutdownMode: Gracefull,

		flipID:  NewFlipID(),
		courier: newCourier(),
		logger:  NoopLogger{},
		mailbox: newMailbox(),
	}
	for _, opt := range opts {
		if err := opt(ctx); err != nil {
			return nil, err
		}
	}
	return ctx, nil
}

func (c *Gronos) done() {
	c.wg.Done()
	c.running--
	if c.running == 0 {
		c.finished = true
	}
}

func (c *Gronos) accumuluate() {
	c.wg.Add(1)
	c.running++
}

func (s *Gronos) remove(id uint) {
	s.runtimes.Delete(id)
}

func (c *Gronos) getID(name string) uint {
	if id, ok := c.runtimesNames.Get(name); ok {
		return *id
	}
	return 0
}

// Run is the bootstrapping function that manages the lifecycle of the application.
func (c *Gronos) Run(ctx context.Context) (*Signal, <-chan error) {

	c.toggleLog = true
	// > but but but why are you doing that?!
	// cause i just want to avoid a stupid instruction interruption
	// if only we had pre-processor directives ¯\_(ツ)_/¯
	if _, ok := c.logger.(NoopLogger); ok {
		c.toggleLog = false
	}

	c.runtimes.ForEach(func(id uint, runtime *RuntimeStation) {
		c.accumuluate()
		go func(r *RuntimeStation) {

			var innerWg sync.WaitGroup

			defer func() {
				innerWg.Done()
				r.courier.Complete()
				r.mailbox.Complete()
				if c.toggleLog {
					c.logger.Info("Gronos runtime wait", slog.Any("id", r.id))
				}
				c.done()
				c.remove(r.id)
				innerWg.Wait()
			}()

			innerWg.Add(1)
			go func() {
				if err := r.runtime(r.ctx, r.mailbox, r.courier, r.signal); err != nil {
					slog.Error("Gronos runtime error", slog.Any("id", r.id), slog.Any("error", err))
					c.courier.Transmit(err)
				}
				if c.toggleLog {
					c.logger.Info("Gronos runtime done", slog.Any("id", r.id))
				}
			}()

			for {
				select {
				case notice := <-r.courier.readNotices():
					c.courier.Transmit(notice)
					if c.toggleLog {
						c.logger.Info("gronos received runtime notice: ", slog.Any("notice", notice))
					}
				case msg := <-r.courier.readMails():
					if c.toggleLog {
						c.logger.Info("gronos received runtime msg: ", slog.Any("msg", msg))
					}
					c.courier.Deliver(msg)
				case <-r.ctx.Done():
					if c.toggleLog {
						c.logger.Info("Gronos context runtime done", slog.Any("id", r.id))
					}
					return
				case <-r.signal.c:
					return
				case <-c.shutdown.Await():
					if c.toggleLog {
						c.logger.Info("Gronos shutdown runtime", slog.Any("id", r.id))
					}
					r.signal.Complete()
					return
				}
			}
		}(runtime)
	})

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		// run all the time
		for {
			select {
			case <-ctx.Done():
				if c.toggleLog {
					c.logger.Info("gronos context done")
				}
				sigCh <- syscall.SIGINT
			case msg := <-c.courier.readMails():
				if c.toggleLog {
					c.logger.Info("gronos deliverying msg: ", slog.Any("msg", msg))
				}
				if runtime, ok := c.runtimes.Get(msg.to); ok {
					runtime.mailbox.post(msg)
				} else {
					if c.toggleLog {
						c.logger.Info("gronos deliverying msg: ", slog.Any("msg", msg), slog.Any("error", "receiver not found"))
					}
				}
			case <-c.shutdown.Await():
				if c.toggleLog {
					c.logger.Info("gronos courrier shutdown")
				}
				c.courier.Complete()
				return
			}
		}
	}()

	// gracefull shutdown
	go func() {
		<-sigCh
		switch c.shutdownMode {
		case Gracefull:
			if c.toggleLog {
				c.logger.Info("Gracefull shutdown")
			}
			c.Kill()
			c.Cut()
			c.Wait()
		case Immediate:
			if c.toggleLog {
				c.logger.Info("Immediate shutdown")
			}
			c.Kill()
			c.Cut()
		}
	}()

	return c.shutdown, c.courier.c
}

// Close all lifelines while waiting
func (c *Gronos) Shutdown() {
	c.Kill()
	c.Cut()
	c.Wait()
}

// Close lifeline
func (c *Gronos) Kill() {
	c.shutdown.Complete()
}

// Close receiver
func (c *Gronos) Cut() {
	c.courier.Complete()
}

// Gracefully wait for the end
func (c *Gronos) Wait() {
	c.wg.Wait()
}

/// TODO: since a terminology for sending errors instead of "Notify"

// func (c *Gronos) NotifyAll(err error) {
// 	for _, runtime := range c.runtimes {
// 		runtime.courier.Transmit(err)
// 	}
// }

// func (c *Gronos) NotifyAllExcept(err error, except uint) {
// 	for _, runtime := range c.runtimes {
// 		if runtime.id != except {
// 			runtime.courier.Transmit(err)
// 		}
// 	}
// }

// func (c *Gronos) NotifyAllExceptAll(err error, excepts ...uint) {
// 	for _, runtime := range c.runtimes {
// 		for _, except := range excepts {
// 			if runtime.id != except {
// 				runtime.courier.Transmit(err)
// 			}
// 		}
// 	}
// }

// func (c *Gronos) SendBroadcastExcept(msg Message, except uint) {
// 	for _, runtime := range c.runtimes {
// 		if runtime.id != except {
// 			runtime.courier.Deliver(envelope{to: runtime.id, Msg: msg})
// 		}
// 	}
// }

// func (c *Gronos) SendBroadcastExceptAll(msg Message, excepts ...uint) {
// 	for _, runtime := range c.runtimes {
// 		for _, except := range excepts {
// 			if runtime.id != except {
// 				runtime.courier.Deliver(envelope{to: runtime.id, Msg: msg})
// 			}
// 		}
// 	}
// }

// func (c *Gronos) CancelAllExcept(except uint) {
// 	for _, runtime := range c.runtimes {
// 		if runtime.id != except {
// 			runtime.cancel()
// 		}
// 	}
// }

// func (c *Gronos) CancelAllExceptAll(excepts ...uint) {
// 	for _, runtime := range c.runtimes {
// 		for _, except := range excepts {
// 			if runtime.id != except {
// 				runtime.cancel()
// 			}
// 		}
// 	}
// }
