package gronos

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"
)

// TODO increase i/o with circular buffers and configurable channel sizes (how to manage full buffer?)
// TODO make a good documentation

type playPauseContext struct {
	context.Context
	done   chan struct{}
	pause  chan struct{}
	resume chan struct{}
}

type Pause <-chan struct{}
type Play <-chan struct{}

type pauseKey string

func WithPlayPause(parent context.Context) (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(parent)
	done := make(chan struct{})
	pause := make(chan struct{}, 1)
	resume := make(chan struct{}, 1)

	playPauseCtx := &playPauseContext{ctx, done, pause, resume}
	ctx = context.WithValue(ctx, pauseKey("pause"), playPauseCtx)

	return ctx, cancel
}

func Paused(ctx context.Context) (Pause, Play, bool) {
	playPauseCtx, ok := ctx.Value(pauseKey("pause")).(*playPauseContext)
	if !ok {
		return nil, nil, false
	}
	return Pause(playPauseCtx.pause), Play(playPauseCtx.resume), true
}

func PlayPauseOperations(ctx context.Context) (func(), func(), bool) {
	playPauseCtx, ok := ctx.Value(pauseKey("pause")).(*playPauseContext)
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
		current: 0,
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

type Envelope struct {
	To  uint
	Msg Message
}

// It's a Mailbox
type Mailbox struct {
	closed bool
	r      chan Envelope
	// TODO: add optional circular buffer
}

func (s *Mailbox) Read() <-chan Envelope {
	return s.r
}

func (r *Mailbox) post(msg Envelope) {
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
func newObserver() *Mailbox {
	return &Mailbox{
		r: make(chan Envelope),
	}
}

// Courier is responsible for delivering error messages. When an error occurs, it's like the Courier "picks up" the error and "delivers" to the central mail station
type Courier struct {
	closed bool
	c      chan error
	e      chan Envelope

	// TODO: add optional circular buffer
}

func (s *Courier) readNotices() <-chan error {
	return s.c
}

func (s *Courier) readMails() <-chan Envelope {
	return s.e
}

func (s *Courier) Deliver(env Envelope) {
	if s.closed {
		// slog.Info("Courier closed")
		return
	}
	if env.Msg != nil {
		s.e <- env
	}
}

func (s *Courier) Notify(err error) {
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
	e := make(chan Envelope, 1) // should be 3 modes: unbuffered, buffered, with 1 buffered channel to prevent panic on multiple ctrl+c signals
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
	runtimes     map[uint]RuntimeStation
	shutdown     *Signal
	wg           sync.WaitGroup
	finished     bool
	running      uint
	flipID       *FlipID
	courier      *Courier
	logger       Logger
	toggleLog    bool
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

func WithCancel() OptionRuntime {
	return func(r *RuntimeStation) error {
		ctx, cnfn := context.WithCancel(r.ctx)
		r.cancel = cnfn
		r.ctx = ctx
		return nil
	}
}

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

func (c *Gronos) Add(opts ...OptionRuntime) (uint, context.CancelFunc) {
	id := c.flipID.Next()
	r := RuntimeStation{
		id:      id,
		courier: newCourier(),
		mailbox: newObserver(),
		signal:  newSignal(),
	}
	ctx := context.Background()
	r.ctx, r.cancel = WithPlayPause(ctx)
	for _, opt := range opts {
		opt(&r)
	}
	c.runtimes[id] = r
	return id, r.cancel
}

func (c *Gronos) AddFuture() uint {
	return c.flipID.Next()
}

func (c *Gronos) Push(id uint, opts ...OptionRuntime) (uint, context.CancelFunc) {
	r := RuntimeStation{
		id:      id,
		courier: newCourier(),
		mailbox: newObserver(),
		signal:  newSignal(),
	}
	ctx := context.Background()
	r.ctx, r.cancel = WithPlayPause(ctx)
	for _, opt := range opts {
		opt(&r)
	}
	c.runtimes[id] = r
	return id, r.cancel
}

func (c *Gronos) Send(msg Message, to uint) {
	if _, ok := c.runtimes[to]; ok {
		c.runtimes[to].courier.Deliver(Envelope{To: to, Msg: msg})
	}
}

func (c *Gronos) Notify(err error, to uint) {
	if _, ok := c.runtimes[to]; ok {
		c.runtimes[to].courier.Notify(err)
	}
}

func (c *Gronos) NotifyAll(err error) {
	for _, runtime := range c.runtimes {
		runtime.courier.Notify(err)
	}
}

func (c *Gronos) SendAll(msg Message) {
	for _, runtime := range c.runtimes {
		runtime.courier.Deliver(Envelope{To: runtime.id, Msg: msg})
	}
}

func (c *Gronos) SendAllExcept(msg Message, except uint) {
	for _, runtime := range c.runtimes {
		if runtime.id != except {
			runtime.courier.Deliver(Envelope{To: runtime.id, Msg: msg})
		}
	}
}

func (c *Gronos) SendAllExceptAll(msg Message, excepts ...uint) {
	for _, runtime := range c.runtimes {
		for _, except := range excepts {
			if runtime.id != except {
				runtime.courier.Deliver(Envelope{To: runtime.id, Msg: msg})
			}
		}
	}
}

func (c *Gronos) NotifyAllExcept(err error, except uint) {
	for _, runtime := range c.runtimes {
		if runtime.id != except {
			runtime.courier.Notify(err)
		}
	}
}

func (c *Gronos) NotifyAllExceptAll(err error, excepts ...uint) {
	for _, runtime := range c.runtimes {
		for _, except := range excepts {
			if runtime.id != except {
				runtime.courier.Notify(err)
			}
		}
	}
}

// Cancel will stop the runtime immediately, your runtime will eventually trigger it's own shutdown
func (c *Gronos) Cancel(id uint) {
	if _, ok := c.runtimes[id]; ok {
		c.runtimes[id].cancel()
	}
}

func (c *Gronos) CancelAllExcept(except uint) {
	for _, runtime := range c.runtimes {
		if runtime.id != except {
			runtime.cancel()
		}
	}
}

func (c *Gronos) CancelAllExceptAll(excepts ...uint) {
	for _, runtime := range c.runtimes {
		for _, except := range excepts {
			if runtime.id != except {
				runtime.cancel()
			}
		}
	}
}

func (c *Gronos) CancelAll() {
	for _, runtime := range c.runtimes {
		runtime.cancel()
	}
}

func (c *Gronos) Pause(id uint) {
	if _, ok := c.runtimes[id]; ok {
		pause, _, ok := PlayPauseOperations(c.runtimes[id].ctx)
		if ok {
			pause()
		}
	}
}

func (c *Gronos) Resume(id uint) {
	if _, ok := c.runtimes[id]; ok {
		_, resume, ok := PlayPauseOperations(c.runtimes[id].ctx)
		if ok {
			resume()
		}
	}
}

func (c *Gronos) Complete(id uint) {
	if _, ok := c.runtimes[id]; ok {
		c.runtimes[id].signal.Complete()
	}
}

func (c *Gronos) PhasingOut(id uint) {
	if _, ok := c.runtimes[id]; ok {
		pause, _, ok := PlayPauseOperations(c.runtimes[id].ctx)
		if ok {
			pause()
		}
		c.runtimes[id].signal.Complete()
		<-c.runtimes[id].signal.Await()
	}
}

func New(opts ...Option) (*Gronos, error) {
	ctx := &Gronos{
		runtimes:     make(map[uint]RuntimeStation),
		shutdown:     newSignal(),
		finished:     false,
		running:      0,
		shutdownMode: Gracefull,
		// receiver:     make(chan error, 1), // Buffered channel to prevent panic on multiple ctrl+c signals
		flipID:  NewFlipID(),
		courier: newCourier(),
		logger:  NoopLogger{},
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
	delete(s.runtimes, id)
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

	for _, runtime := range c.runtimes {
		c.accumuluate()
		go func(r RuntimeStation) {

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
					c.courier.Notify(err)
				}
				if c.toggleLog {
					c.logger.Info("Gronos runtime done", slog.Any("id", r.id))
				}
			}()

			for {
				select {
				case notice := <-r.courier.readNotices():
					c.courier.Notify(notice)
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
	}

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)

	go func() {
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
				if _, ok := c.runtimes[msg.To]; ok {
					c.runtimes[msg.To].mailbox.post(msg)
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

// Cron is a function that runs a function periodically.
func Cron(duration time.Duration, fn func() error) RuntimeFunc {
	return func(ctx context.Context, mailbox *Mailbox, courier *Courier, shutdown *Signal) error {

		ticker := time.NewTicker(duration)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return nil
			case <-shutdown.Await():
				return nil
			case <-ticker.C:
				go func() {
					if err := fn(); err != nil {
						courier.Notify(err)
						select {
						case <-ctx.Done():
							return
						case <-shutdown.Await():
							return
						}
					}
				}()
			}
		}

		return nil
	}
}

// Timed is a function that runs a function periodically and waits for it to complete.
func Timed(duration time.Duration, fn func() error) RuntimeFunc {
	return func(ctx context.Context, mailbox *Mailbox, courier *Courier, shutdown *Signal) error {

		ticker := time.NewTicker(duration)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return nil
			case <-shutdown.Await():
				return nil
			case <-ticker.C:
				if err := fn(); err != nil {
					courier.Notify(err)
					select {
					case <-ctx.Done():
						return nil
					case <-shutdown.Await():
						return nil
					}
				}
			}
		}

		return nil
	}
}
