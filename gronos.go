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
}

func (s *Courier) readNotices() <-chan error {
	return s.c
}

func (s *Courier) readMails() <-chan Envelope {
	return s.e
}

func (s *Courier) Deliver(env Envelope) {
	if s.closed {
		slog.Info("Courier closed")
		return
	}
	s.e <- env
}

func (s *Courier) Notify(err error) {
	if s.closed {
		slog.Info("Courier closed")
		return
	}
	s.c <- err
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
	c := make(chan error)
	e := make(chan Envelope)
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

type Receiver chan error

// RuntimeFunc represents a function that runs a runtime.
type RuntimeFunc func(ctx context.Context, mailbox *Mailbox, courrier *Courier, shutdown *Signal) error

// Centralized place that manage the lifecycle of runtimes
type Station struct {
	shutdownMode Shutdown
	runtimes     map[uint]RuntimeStation
	shutdown     *Signal
	wg           sync.WaitGroup
	finished     bool
	running      uint
	errChan      Receiver
}

type Option func(*Station) error

// Interuption won't wait for runtimes to gracefully finish
func WithImmediateShutdown() Option {
	return func(c *Station) error {
		c.shutdownMode = Immediate
		return nil
	}
}

// Interuption will wait for runtimes to gracefully finish
func WithGracefullShutdown() Option {
	return func(c *Station) error {
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

func (c *Station) Add(opts ...OptionRuntime) (uint, context.CancelFunc) {
	id := uint(len(c.runtimes))
	r := RuntimeStation{
		id:      id,
		ctx:     context.Background(), // new context
		courier: newCourier(),
		mailbox: newObserver(),
	}
	r.ctx, r.cancel = context.WithCancel(r.ctx) // basic one
	for _, opt := range opts {
		opt(&r)
	}
	c.runtimes[id] = r
	return id, r.cancel
}

func New(opts ...Option) (*Station, error) {
	ctx := &Station{
		runtimes:     make(map[uint]RuntimeStation),
		shutdown:     newSignal(),
		finished:     false,
		running:      0,
		shutdownMode: Gracefull,
		errChan:      make(chan error, 1), // Buffered channel to prevent panic on multiple ctrl+c signals
	}
	for _, opt := range opts {
		if err := opt(ctx); err != nil {
			return nil, err
		}
	}
	return ctx, nil
}

func (c *Station) done() {
	c.wg.Done()
	c.running--
	if c.running == 0 {
		c.finished = true
	}
}

func (c *Station) accumuluate() {
	c.wg.Add(1)
	c.running++
}

// Run is the bootstrapping function that manages the lifecycle of the application.
func (c *Station) Run() (*Signal, Receiver) {
	// ctx, cancel := context.WithCancel(context.Background())
	// defer cancel()

	courrier := newCourier()

	for _, runtime := range c.runtimes {
		c.accumuluate()
		go func(r RuntimeStation) {

			var innerWg sync.WaitGroup
			innerLifeline := newSignal()

			defer func() {
				r.courier.Complete()
				r.mailbox.Complete()
				slog.Info("Gronos runtime wait", slog.Any("id", r.id))
				innerWg.Wait()
				slog.Info("Gronos runtime finished", slog.Any("id", r.id))
				c.done()
			}()

			innerWg.Add(1)
			go func() {
				r.runtime(r.ctx, r.mailbox, r.courier, innerLifeline)
				slog.Info("Gronos runtime done", slog.Any("id", r.id))
				innerWg.Done()
				r.courier.Complete()
				r.mailbox.Complete()
			}()

			for {
				select {
				case notice := <-r.courier.readNotices():
					slog.Info("gronos received runtime notice: ", slog.Any("notice", notice))
				case msg := <-r.courier.readMails():
					slog.Info("gronos received runtime msg: ", slog.Any("msg", msg))
					courrier.Deliver(msg)
				case <-r.ctx.Done():
					r.courier.Complete()
					r.mailbox.Complete()
					slog.Info("Gronos context runtime done", slog.Any("id", r.id))
					return
				case <-c.shutdown.Await():
					r.courier.Complete()
					r.mailbox.Complete()
					slog.Info("Gronos shutdown runtime", slog.Any("id", r.id))
					innerLifeline.Complete()
					return
				case err := <-r.courier.c:
					c.errChan <- err
				}
			}
		}(runtime)
	}

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		for {
			select {
			case err := <-courrier.readNotices():
				slog.Info("gronos deliverying notice: ", slog.Any("notice", err))
			case msg := <-courrier.readMails():
				slog.Info("gronos deliverying msg: ", slog.Any("msg", msg))
				if _, ok := c.runtimes[msg.To]; ok {
					c.runtimes[msg.To].mailbox.post(msg)
				} else {
					slog.Info("gronos deliverying msg: ", slog.Any("msg", msg), slog.Any("error", "receiver not found"))
				}
			case <-c.shutdown.Await():
				slog.Info("gronos courrier shutdown")
				courrier.Complete()
				return
			}
		}
	}()

	// gracefull shutdown
	// TODO make it configurable
	go func() {
		<-sigCh
		switch c.shutdownMode {
		case Gracefull:
			slog.Info("Gracefull shutdown")
			// cancel()
			c.Kill()
			c.Cut()
			// calling ourselves to wait for the end
			c.Wait()
		case Immediate:
			slog.Info("Immediate shutdown")
			// cancel()
			c.Kill()
			c.Cut()
		}
	}()

	return c.shutdown, c.errChan
}

// Close all lifelines while waiting
func (c *Station) Shutdown() {
	c.Kill()
	c.Cut()
	c.Wait()
}

// Close lifeline
func (c *Station) Kill() {
	c.shutdown.Complete()
}

// Close receiver
func (c *Station) Cut() {
	close(c.errChan)
}

// Gracefully wait for the end
func (c *Station) Wait() {
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
