package nonos

import (
	"context"
	"fmt"
	"runtime"
	"sync"
	"time"

	"github.com/avast/retry-go/v3"
)

type RuntimeApplication func(ctx context.Context, shutdown <-chan struct{}) error

type message interface{}

type gronos[K comparable] struct {
	applications sync.Map
	keys         ConcurrentArray[K]
	com          chan message
	wait         sync.WaitGroup
	ctx          context.Context
}

type DeadLetter[K comparable] struct {
	Key    K
	Reason error
}

type Add[K comparable] struct {
	Key K
	App RuntimeApplication
}

type applicationContext[K comparable] struct {
	k        K
	app      RuntimeApplication
	ctx      context.Context
	com      chan message
	retries  uint
	alive    bool
	reason   error
	shutdown chan struct{}
	closer   func() // proxy function to close the channel shutdown
}

// todo add options
func New[K comparable](ctx context.Context, init map[K]RuntimeApplication) *gronos[K] {
	g := &gronos[K]{
		com:  make(chan message, 200), // todo add option
		keys: ConcurrentArray[K]{},
		ctx:  ctx,
	}
	for k, v := range init {
		g.Add(k, v)
	}
	return g
}

func (g *gronos[K]) Start() chan error {
	return g.run(g.ctx)
}

func (g *gronos[K]) Shutdown() {
	g.applications.Range(func(key, value any) bool {
		app := value.(applicationContext[K])
		if app.alive {
			app.alive = false
			g.com <- DeadLetter[K]{Key: key.(K), Reason: fmt.Errorf("shutdown")}
		}
		return true
	})
}

func (g *gronos[K]) Wait() {
	g.wait.Wait()
	close(g.com) // Close the channel after all applications have finished
}

func (g *gronos[K]) run(ctx context.Context) chan error {
	cerr := make(chan error)
	go func() {
		closer := sync.OnceFunc(func() {
			close(cerr)
		})
		defer closer()
		for {
			select {
			case <-ctx.Done():
				g.drainMessages(cerr) // Drain remaining messages
				return
			case m, ok := <-g.com:
				if !ok {
					// Channel closed, exit the goroutine
					return
				}
				g.handleMessage(m, cerr)
			}
			runtime.Gosched()
		}
	}()
	return cerr
}

func (g *gronos[K]) handleMessage(m message, cerr chan<- error) {
	switch msg := m.(type) {
	case DeadLetter[K]:
		var value any
		var app applicationContext[K]
		var ok bool
		if value, ok = g.applications.Load(msg.Key); !ok {
			return
		}
		if app, ok = value.(applicationContext[K]); !ok {
			return
		}
		app.alive = false
		app.reason = msg.Reason
		app.closer()
		g.applications.Store(app.k, app)
		g.wait.Done()
		if msg.Reason != nil && msg.Reason.Error() != "shutdown" {
			cerr <- fmt.Errorf("app error %v %v", msg.Key, msg.Reason)
		}
	case Add[K]:
		if err := g.Add(msg.Key, msg.App); err != nil {
			cerr <- err
		}
	default:
	}
}

func (g *gronos[K]) drainMessages(cerr chan<- error) {
	for {
		select {
		case m, ok := <-g.com:
			if !ok {
				// Channel closed, all messages processed
				return
			}
			g.handleMessage(m, cerr)
		}
	}
}

type ctxKey string

var comKey ctxKey = "com"

func (g *gronos[K]) createContext() context.Context {
	ctx := context.Background()
	ctx = context.WithValue(ctx, comKey, g.com)
	return ctx
}

func (g *gronos[K]) Add(k K, v RuntimeApplication) error {
	if _, ok := g.applications.Load(k); ok {
		return fmt.Errorf("application with key %v already exists", k)
	}
	appCtx := applicationContext[K]{
		k:        k,
		app:      v,
		ctx:      g.createContext(),
		com:      g.com,
		retries:  0,
		shutdown: make(chan struct{}),
		reason:   nil,
		alive:    true,
	}

	appCtx.closer = sync.OnceFunc(func() {
		close(appCtx.shutdown)
	})

	g.applications.Store(k, appCtx)
	g.keys.Append(k)
	g.wait.Add(1)

	go func(key K, ctx applicationContext[K]) {
		var err error
		if ctx.retries == 0 {
			err = ctx.app(ctx.ctx, ctx.shutdown)
		} else {
			err = retry.Do(func() error {
				return ctx.app(ctx.ctx, ctx.shutdown)
			}, retry.Attempts(ctx.retries))
		}
		if err != nil {
			g.com <- DeadLetter[K]{Key: key, Reason: err}
		}
	}(k, appCtx)

	return nil
}

func UseBus(ctx context.Context) (chan<- message, error) {
	value := ctx.Value(comKey)
	if value == nil {
		return nil, fmt.Errorf("com not found in context")
	}
	return value.(chan message), nil
}

type TickingApplication func(context.Context) error

type tickerWrapper struct {
	clock *Clock
	ctx   context.Context
	app   TickingApplication
	cerr  chan error
}

func (t tickerWrapper) Tick() {
	if err := t.app(t.ctx); err != nil {
		t.clock.Stop()
		t.cerr <- err
	}
}

func Worker(interval time.Duration, mode ExecutionMode, app TickingApplication) RuntimeApplication {
	return func(ctx context.Context, shutdown <-chan struct{}) error {
		w := &tickerWrapper{
			clock: NewClock(WithInterval(interval)),
			ctx:   ctx,
			app:   app,
			cerr:  make(chan error, 1),
		}
		w.clock.Add(w, mode)
		w.clock.Start()

		select {
		case <-ctx.Done():
			w.clock.Stop()
			return ctx.Err()
		case <-shutdown:
			w.clock.Stop()
			return nil
		case err := <-w.cerr:
			w.clock.Stop()
			return err
		}
	}
}
