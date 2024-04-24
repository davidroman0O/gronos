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

type Sink struct {
	closed bool
	c      chan error
}

func (s *Sink) Write(err error) {
	if s.closed {
		return
	}
	s.c <- err
}

func (s *Sink) Close() {
	if s.closed {
		return
	}
	close(s.c)
	s.closed = true
}

func newSink() *Sink {
	c := make(chan error)
	return &Sink{
		c: c,
	}
}

type Lifeline struct {
	closed bool
	c      chan struct{}
}

func (s *Lifeline) Close() {
	if s.closed {
		return
	}
	close(s.c)
	s.closed = true
}

func (s *Lifeline) Wait() <-chan struct{} {
	return s.c
}

func newLifeline() *Lifeline {
	c := make(chan struct{})
	return &Lifeline{
		c: c,
	}
}

type Receiver chan error

// RuntimeFunc represents a function that runs a runtime.
type RuntimeFunc func(ctx context.Context, shutdown *Lifeline, err *Sink) error

type Context struct {
	shutdownMode Shutdown
	runtimes     map[uint]RuntimeContext
	shutdown     *Lifeline
	wg           sync.WaitGroup
	finished     bool
	running      uint
	errChan      Receiver
}

type Option func(*Context) error

// Interuption won't wait for runtimes to gracefully finish
func WithImmediateShutdown() Option {
	return func(c *Context) error {
		c.shutdownMode = Immediate
		return nil
	}
}

// Interuption will wait for runtimes to gracefully finish
func WithGracefullShutdown() Option {
	return func(c *Context) error {
		c.shutdownMode = Gracefull
		return nil
	}
}

type RuntimeContext struct {
	id      uint
	ctx     context.Context
	runtime RuntimeFunc
	cancel  context.CancelFunc
	sink    *Sink
}

// not sure if i'm going to use the error here
type OptionRuntime func(*RuntimeContext) error

func WithContext(ctx context.Context) OptionRuntime {
	return func(r *RuntimeContext) error {
		r.ctx = ctx
		return nil
	}
}

func WithTimeout(d time.Duration) OptionRuntime {
	return func(r *RuntimeContext) error {
		ctx, cnfn := context.WithTimeout(r.ctx, d)
		r.cancel = cnfn
		r.ctx = ctx
		return nil
	}
}

func WithValue(key, value interface{}) OptionRuntime {
	return func(r *RuntimeContext) error {
		r.ctx = context.WithValue(r.ctx, key, value)
		return nil
	}
}

func WithCancel() OptionRuntime {
	return func(r *RuntimeContext) error {
		ctx, cnfn := context.WithCancel(r.ctx)
		r.cancel = cnfn
		r.ctx = ctx
		return nil
	}
}

func WithDeadline(d time.Time) OptionRuntime {
	return func(r *RuntimeContext) error {
		ctx, cfn := context.WithDeadline(r.ctx, d)
		r.cancel = cfn
		r.ctx = ctx
		return nil
	}
}

func WithRuntime(r RuntimeFunc) OptionRuntime {
	return func(rc *RuntimeContext) error {
		rc.runtime = r
		return nil
	}
}

func (c *Context) Add(opts ...OptionRuntime) (uint, context.CancelFunc) {
	id := uint(len(c.runtimes))
	r := RuntimeContext{
		id:   id,
		ctx:  context.Background(), // new context
		sink: newSink(),
	}
	r.ctx, r.cancel = context.WithCancel(r.ctx) // basic one
	for _, opt := range opts {
		opt(&r)
	}
	c.runtimes[id] = r
	return id, r.cancel
}

func New(opts ...Option) (*Context, error) {
	ctx := &Context{
		runtimes:     make(map[uint]RuntimeContext),
		shutdown:     newLifeline(),
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

func (c *Context) done() {
	c.wg.Done()
	c.running--
	if c.running == 0 {
		c.finished = true
	}
}

func (c *Context) accumuluate() {
	c.wg.Add(1)
	c.running++
}

// Run is the bootstrapping function that manages the lifecycle of the application.
func (c *Context) Run() (*Lifeline, Receiver) {
	// ctx, cancel := context.WithCancel(context.Background())
	// defer cancel()

	for _, runtime := range c.runtimes {
		c.accumuluate()
		go func(r RuntimeContext) {

			var innerWg sync.WaitGroup
			innerLifeline := newLifeline()

			defer func() {
				r.sink.Close()
				slog.Info("Gronos runtime wait", slog.Any("id", r.id))
				innerWg.Wait()
				slog.Info("Gronos runtime finished", slog.Any("id", r.id))
				c.done()
			}()

			innerWg.Add(1)
			go func() {
				r.runtime(r.ctx, innerLifeline, r.sink)
				slog.Info("Gronos runtime done", slog.Any("id", r.id))
				innerWg.Done()
			}()

			for {
				select {
				case <-r.ctx.Done():
					slog.Info("Gronos context runtime done", slog.Any("id", r.id))
					return
				case <-c.shutdown.Wait():
					slog.Info("Gronos shutdown runtime", slog.Any("id", r.id))
					innerLifeline.Close()
					return
				case err := <-r.sink.c:
					c.errChan <- err
				}
			}
		}(runtime)
	}

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)

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
func (c *Context) Shutdown() {
	c.Kill()
	c.Cut()
	c.Wait()
}

// Close lifeline
func (c *Context) Kill() {
	c.shutdown.Close()
}

// Close receiver
func (c *Context) Cut() {
	close(c.errChan)
}

// Gracefully wait for the end
func (c *Context) Wait() {
	c.wg.Wait()
}

// Cron is a function that runs a function periodically.
func Cron(duration time.Duration, fn func() error) RuntimeFunc {
	return func(ctx context.Context, shutdown *Lifeline, sink *Sink) error {

		ticker := time.NewTicker(duration)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return nil
			case <-shutdown.Wait():
				return nil
			case <-ticker.C:
				go func() {
					if err := fn(); err != nil {
						sink.Write(err)
						select {
						case <-ctx.Done():
							return
						case <-shutdown.Wait():
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
	return func(ctx context.Context, shutdown *Lifeline, sink *Sink) error {

		ticker := time.NewTicker(duration)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return nil
			case <-shutdown.Wait():
				return nil
			case <-ticker.C:
				if err := fn(); err != nil {
					sink.Write(err)
					select {
					case <-ctx.Done():
						return nil
					case <-shutdown.Wait():
						return nil
					}
				}
			}
		}

		return nil
	}
}
