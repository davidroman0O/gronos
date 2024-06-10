package gronos

import (
	"context"
	"log/slog"
	"time"

	"github.com/davidroman0O/gronos/clock"
	"github.com/davidroman0O/gronos/logging"
)

/// Gronos organize the lifecycle of runtimes to help you compose your applications.
/// See runtimes and channels as gears, we're building a digital machine

/// Features
/// - basic runtimes functions
/// - play/pause context
/// - communication between runtimes
/// - router that dispatch and analyze messages

// TODO Replace API with ID and use string for names
// allowing both ID and name
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

type gronosKey string

type ShutdownMode uint

const (
	Gracefull ShutdownMode = iota
	Immediate
)

// Centralized place that manage the lifecycle of runtimes
type Gronos struct {
	logger logging.Logger

	router *Router

	shutdown     *Signal
	shutdownMode ShutdownMode
	finished     bool

	courier *Courier // someone might want to send messages to all runtimes

	toggleLog bool

	// Clock of the router
	// The router will have the clock of the registry
	// It's a hierarchy of clocks, each system control the next one
	clock *clock.Clock
}

type Option func(*Gronos) error

// Interuption won't wait for runtimes to gracefully finish
func WithImmediateShutdown() Option {
	return func(c *Gronos) error {
		c.shutdownMode = Immediate
		return nil
	}
}

func WithLogger(logger logging.Logger) Option {
	return func(c *Gronos) error {
		c.logger = logger
		return nil
	}
}

func WithSlogLogger() Option {
	return func(c *Gronos) error {
		c.logger = logging.NewSlog()
		return nil
	}
}

func WithFmtLogger() Option {
	return func(c *Gronos) error {
		c.logger = logging.NewFmt()
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

func (c *Gronos) Add(name string, opts ...OptionRuntime) (uint, context.CancelFunc) {
	return c.router.Add(name, opts...)
}

// // `Direct` require the ID of the runtime, faster but less readable
// func (c *Gronos) Direct(msg Message, to uint) error {
// 	return c.router.Direct(msg, to)
// }

// // `Delivery` require the name of the runtime, more readable but slower
// func (c *Gronos) Named(msg Message, name string) error {
// 	return c.router.named(msg, name)
// }

// func (c *Gronos) Broadcast(msg Message) error {
// 	return c.router.broadcast(msg)
// }

// func (c *Gronos) Transmit(err error, to uint) error {
// 	return c.router.transmit(err, to)
// }

// // Cancel will stop the runtime immediately, your runtime will eventually trigger it's own shutdown
// func (c *Gronos) Cancel(id uint) error {
// 	return c.router.cancel(id)
// }

// func (c *Gronos) CancelAll() error {
// 	return c.router.cancelAll()
// }

// func (c *Gronos) DirectPause(id uint) error {
// 	return c.router.directPause(id)
// }

// func (c *Gronos) DirectResume(id uint) error {
// 	return c.router.directResume(id)
// }

// func (c *Gronos) DirectComplete(id uint) error {
// 	return c.router.directComplete(id)
// }

// func (c *Gronos) DirectPhasingOut(id uint) error {
// 	return c.router.directPhasingOut(id)
// }

func New(opts ...Option) (*Gronos, error) {
	ctx := &Gronos{
		// runtimes:      newSafeMapPtr[uint, RuntimeStation](), // faster usage - dev could use only that to be faster
		// runtimesNames: newSafeMapPtr[string, uint](),         // convienient usage - dev could use that to be more readable

		shutdownMode: Gracefull,
		shutdown:     newSignal(),
		finished:     false,
		logger:       logging.NoopLogger{},
		clock:        clock.New(clock.WithInterval(time.Millisecond * 100)),

		// flipID: NewFlipID(),

		router: newRouter(), // todo pass options

		// courier: newCourier(),
		// mailbox: newMailbox(),
	}
	for _, opt := range opts {
		if err := opt(ctx); err != nil {
			return nil, err
		}
	}

	// the Gronos clock will trigger the router ticker
	ctx.clock.Add(ctx.router, clock.ManagedTimeline)

	return ctx, nil
}

func (g *Gronos) Run(ctx context.Context) (*Signal, <-chan error) {

	g.clock.Start()        // start the clock of the router
	g.router.clock.Start() // start the clock of the runtime form the router

	go func() {
		select {
		case <-ctx.Done():
			slog.Info("shutting down gronos")
			<-g.router.registry.shutdown.Await()
			<-g.router.shutdown.Await()
			// todo stop everything else
			g.clock.Stop()
			g.router.clock.Stop()
			g.shutdown.Complete()
		}
	}()

	return g.shutdown, nil
}

func (g *Gronos) GracefullWait() <-chan struct{} {
	return g.shutdown.Await()
}

// func (c *Gronos) done() {
// 	// c.wg.Done()
// 	// c.running--
// 	// if c.running == 0 {
// 	// 	c.finished = true
// 	// }
// }

// func (c *Gronos) accumuluate() {
// 	// c.wg.Add(1)
// 	// c.running++
// }

// Run is the bootstrapping function that manages the lifecycle of the application.
// TODO replace with Router
// func (c *Gronos) Run(ctx context.Context) (*Signal, <-chan error) {

// 	c.toggleLog = true
// 	// > but but but why are you doing that?!
// 	// cause i just want to avoid a stupid instruction interruption
// 	// if only we had pre-processor directives ¯\_(ツ)_/¯
// 	if _, ok := c.logger.(NoopLogger); ok {
// 		c.toggleLog = false
// 	}

// 	c.router.runtimes.ForEach(func(id uint, runtime *RuntimeStation) error {
// 		c.accumuluate()
// 		go func(r *RuntimeStation) {

// 			var innerWg sync.WaitGroup

// 			defer func() {
// 				innerWg.Done()
// 				r.courier.Complete()
// 				r.mailbox.Complete()
// 				if c.toggleLog {
// 					c.logger.Info("Gronos runtime wait", slog.Any("id", r.id))
// 				}
// 				c.done()
// 				c.router.remove(r.id)
// 				innerWg.Wait()
// 			}()

// 			innerWg.Add(1)
// 			go func() {
// 				if err := r.runtime(r.ctx, r.mailbox, r.courier, r.signal); err != nil {
// 					slog.Error("Gronos runtime error", slog.Any("id", r.id), slog.Any("error", err))
// 					c.courier.Transmit(err)
// 				}
// 				if c.toggleLog {
// 					c.logger.Info("Gronos runtime done", slog.Any("id", r.id))
// 				}
// 			}()

// 			for {
// 				select {
// 				case notice := <-r.courier.readNotices():
// 					c.courier.Transmit(notice)
// 					if c.toggleLog {
// 						c.logger.Info("gronos received runtime notice: ", slog.Any("notice", notice))
// 					}
// 				case msg := <-r.courier.readMails():
// 					if c.toggleLog {
// 						c.logger.Info("gronos received runtime msg: ", slog.Any("msg", msg))
// 					}
// 					c.courier.Deliver(msg)
// 				case <-r.ctx.Done():
// 					if c.toggleLog {
// 						c.logger.Info("Gronos context runtime done", slog.Any("id", r.id))
// 					}
// 					return
// 				case <-r.signal.c:
// 					return
// 				case <-c.shutdown.Await():
// 					if c.toggleLog {
// 						c.logger.Info("Gronos shutdown runtime", slog.Any("id", r.id))
// 					}
// 					r.signal.Complete()
// 					return
// 				}
// 			}
// 		}(runtime)
// 		return nil
// 	})

// 	sigCh := make(chan os.Signal, 1)
// 	signal.Notify(sigCh, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)

// 	go func() {
// 		// run all the time
// 		for {
// 			select {
// 			case <-ctx.Done():
// 				if c.toggleLog {
// 					c.logger.Info("gronos context done")
// 				}
// 				sigCh <- syscall.SIGINT
// 			case msg := <-c.courier.readMails():
// 				if c.toggleLog {
// 					c.logger.Info("gronos deliverying msg: ", slog.Any("msg", msg))
// 				}
// 				if runtime, ok := c.router.runtimes.Get(msg.to); ok {
// 					runtime.mailbox.post(msg)
// 				} else {
// 					if c.toggleLog {
// 						c.logger.Info("gronos deliverying msg: ", slog.Any("msg", msg), slog.Any("error", "receiver not found"))
// 					}
// 				}
// 			case <-c.shutdown.Await():
// 				if c.toggleLog {
// 					c.logger.Info("gronos courrier shutdown")
// 				}
// 				c.courier.Complete()
// 				return
// 			}
// 		}
// 	}()

// 	// gracefull shutdown
// 	go func() {
// 		<-sigCh
// 		switch c.shutdownMode {
// 		case Gracefull:
// 			if c.toggleLog {
// 				c.logger.Info("Gracefull shutdown")
// 			}
// 			c.Kill()
// 			c.Cut()
// 			c.Wait()
// 		case Immediate:
// 			if c.toggleLog {
// 				c.logger.Info("Immediate shutdown")
// 			}
// 			c.Kill()
// 			c.Cut()
// 		}
// 	}()

// 	return c.shutdown, c.courier.c
// }

// Close all lifelines while waiting
// func (c *Gronos) Shutdown() {
// 	c.Kill()
// 	c.Cut()
// 	c.Wait()
// }

// // Close lifeline
// func (c *Gronos) Kill() {
// 	c.shutdown.Complete()
// }

// // Close receiver
// func (c *Gronos) Cut() {
// 	c.courier.Complete()
// }

// // Gracefully wait for the end
// func (c *Gronos) Wait() {
// 	// c.wg.Wait()
// }

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
