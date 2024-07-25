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
	cancel       context.CancelFunc
	done         chan struct{}
}

type DeadLetter[K comparable] struct {
	Key    K
	Reason error
}

type Terminated[K comparable] struct {
	Key K
}

type ContextTerminated[K comparable] struct {
	Key K
	Err error
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
	cancel   func() // proxy function to cancel the context
}

func New[K comparable](ctx context.Context, init map[K]RuntimeApplication) *gronos[K] {
	ctx, cancel := context.WithCancel(ctx)
	g := &gronos[K]{
		com:    make(chan message, 200),
		keys:   ConcurrentArray[K]{},
		ctx:    ctx,
		cancel: cancel,
		done:   make(chan struct{}),
	}
	for k, v := range init {
		g.Add(k, v)
	}
	return g
}

func (g *gronos[K]) Start() chan error {
	errChan := make(chan error, 1)
	go g.run(errChan)
	return errChan
}

func (g *gronos[K]) Shutdown() {
	g.cancel()
	select {
	case <-g.done:
		// Shutdown completed successfully
	case <-time.After(5 * time.Second):
		// Timeout occurred, log a warning
		fmt.Println("Warning: Shutdown timed out after 5 seconds")
	}
}

func (g *gronos[K]) Wait() {
	<-g.done
}

func (g *gronos[K]) run(errChan chan<- error) {
	defer close(g.done)
	defer close(errChan)

	for {
		select {
		case <-g.ctx.Done():
			g.shutdownApps()
			errChan <- g.ctx.Err() // Propagate the context error
			return
		case m, ok := <-g.com:
			if !ok {
				return
			}
			if err := g.handleMessage(m); err != nil {
				errChan <- err
			}
		}
		runtime.Gosched()
	}
}

func (g *gronos[K]) shutdownApps() {
	var wg sync.WaitGroup
	g.applications.Range(func(key, value interface{}) bool {
		app := value.(applicationContext[K])
		if app.alive {
			wg.Add(1)
			go func() {
				defer wg.Done()
				app.cancel()
				<-app.shutdown
			}()
		}
		return true
	})
	wg.Wait()
}

func (g *gronos[K]) handleMessage(m message) error {
	switch msg := m.(type) {
	case DeadLetter[K]:
		var value any
		var app applicationContext[K]
		var ok bool
		if value, ok = g.applications.Load(msg.Key); !ok {
			return nil
		}
		if app, ok = value.(applicationContext[K]); !ok {
			return nil
		}
		app.alive = false
		app.reason = msg.Reason
		app.closer()
		g.applications.Store(app.k, app)
		if msg.Reason != nil && msg.Reason.Error() != "shutdown" {
			return fmt.Errorf("app error %v %v", msg.Key, msg.Reason)
		}
	case Terminated[K]:
		var value any
		var app applicationContext[K]
		var ok bool
		if value, ok = g.applications.Load(msg.Key); !ok {
			return nil
		}
		if app, ok = value.(applicationContext[K]); !ok {
			return nil
		}
		app.alive = false
		app.closer()
		g.applications.Store(app.k, app)
	case ContextTerminated[K]:
		var value any
		var app applicationContext[K]
		var ok bool
		if value, ok = g.applications.Load(msg.Key); !ok {
			return nil
		}
		if app, ok = value.(applicationContext[K]); !ok {
			return nil
		}
		app.alive = false
		app.cancel()
		g.applications.Store(app.k, app)
	case Add[K]:
		return g.Add(msg.Key, msg.App)
	}
	return nil
}

func (g *gronos[K]) Add(k K, v RuntimeApplication) error {
	if _, ok := g.applications.Load(k); ok {
		return fmt.Errorf("application with key %v already exists", k)
	}
	if g.ctx.Err() != nil {
		return fmt.Errorf("context is already cancelled")
	}

	ctx, cancelFunc := context.WithCancel(g.ctx)
	shutdown := make(chan struct{})

	appCtx := applicationContext[K]{
		k:        k,
		app:      v,
		ctx:      ctx,
		com:      g.com,
		retries:  0,
		shutdown: shutdown,
		reason:   nil,
		alive:    true,
	}

	appCtx.closer = sync.OnceFunc(func() {
		close(shutdown)
		g.wait.Done()
	})

	appCtx.cancel = sync.OnceFunc(func() {
		cancelFunc()
		appCtx.closer()
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
		if err != nil && err != context.Canceled {
			select {
			case g.com <- DeadLetter[K]{Key: key, Reason: err}:
			case <-g.ctx.Done():
			}
		} else if err == context.Canceled {
			select {
			case g.com <- ContextTerminated[K]{Key: key, Err: ctx.ctx.Err()}:
			case <-g.ctx.Done():
			}
		} else {
			select {
			case g.com <- Terminated[K]{Key: key}:
			case <-g.ctx.Done():
			}
		}
	}(k, appCtx)

	return nil
}

type ctxKey string

var comKey ctxKey = "com"

func (g *gronos[K]) createContext() (context.Context, context.CancelFunc) {
	ctx := context.WithValue(context.Background(), comKey, g.com)
	ctx, cancel := context.WithCancel(ctx)
	return ctx, cancel
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
