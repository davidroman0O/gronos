package gronos

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
)

// CancellableStateTask represents a task that can be cancelled and operates on a state
type CancellableStateTask[T any] func(ctx context.Context, state *T) error

// IteratorStateOption is a function type for configuring the IteratorState
type IteratorStateOption[T any] func(*iteratorStateConfig[T])

type iteratorStateConfig[T any] struct {
	loopOpts []LoopableIteratorStateOption[T]
	state    *T
}

// WithLoopableIteratorStateOptions adds LoopableIteratorStateOptions to the IteratorState
func WithLoopableIteratorStateOptions[T any](opts ...LoopableIteratorStateOption[T]) IteratorStateOption[T] {
	return func(c *iteratorStateConfig[T]) {
		c.loopOpts = append(c.loopOpts, opts...)
	}
}

// WithInitialState sets the initial state for the IteratorState
func WithInitialState[T any](state *T) IteratorStateOption[T] {
	return func(c *iteratorStateConfig[T]) {
		c.state = state
	}
}

// IteratorState creates a RuntimeApplication that uses a LoopableIteratorState to execute tasks with a shared state
func IteratorState[T any](tasks []CancellableStateTask[T], opts ...IteratorStateOption[T]) RuntimeApplication {
	config := &iteratorStateConfig[T]{}
	for _, opt := range opts {
		opt(config)
	}

	return func(ctx context.Context, shutdown <-chan struct{}) error {
		var state *T
		if config.state != nil {
			state = config.state
		} else {
			state = new(T)
		}

		li := NewLoopableIteratorState(tasks, state, config.loopOpts...)
		errChan := li.Run(ctx)

		var finalErr error
		select {
		case <-ctx.Done():
			li.Cancel()
			finalErr = ctx.Err()
		case <-shutdown:
			li.Cancel()
		case err, ok := <-errChan:
			if !ok {
				return nil
			}
			li.Cancel()
			finalErr = err
		}

		li.Wait()

		return finalErr
	}
}

// LoopableIteratorState is similar to LoopableIterator but with a shared state
type LoopableIteratorState[T any] struct {
	tasks       []CancellableStateTask[T]
	state       *T
	index       int
	mu          sync.Mutex
	onInit      func(ctx context.Context, state *T) (context.Context, error)
	onError     func(error) error
	shouldStop  func(error) bool
	beforeLoop  func(ctx context.Context, state *T) error
	afterLoop   func(ctx context.Context, state *T) error
	extraCancel []context.CancelFunc
	running     atomic.Bool
	stopCh      chan struct{}
	ctx         context.Context
	cancel      context.CancelFunc
	wg          sync.WaitGroup
}

type LoopableIteratorStateOption[T any] func(*LoopableIteratorState[T])

func WithExtraCancelState[T any](cancels ...context.CancelFunc) LoopableIteratorStateOption[T] {
	return func(li *LoopableIteratorState[T]) {
		li.extraCancel = append(li.extraCancel, cancels...)
	}
}

func WithOnErrorState[T any](handler func(error) error) LoopableIteratorStateOption[T] {
	return func(li *LoopableIteratorState[T]) {
		li.onError = handler
	}
}

func WithShouldStopState[T any](shouldStop func(error) bool) LoopableIteratorStateOption[T] {
	return func(li *LoopableIteratorState[T]) {
		li.shouldStop = shouldStop
	}
}

func WithBeforeLoopState[T any](beforeLoop func(ctx context.Context, state *T) error) LoopableIteratorStateOption[T] {
	return func(li *LoopableIteratorState[T]) {
		li.beforeLoop = beforeLoop
	}
}

func WithAfterLoopState[T any](afterLoop func(ctx context.Context, state *T) error) LoopableIteratorStateOption[T] {
	return func(li *LoopableIteratorState[T]) {
		li.afterLoop = afterLoop
	}
}

func WithOnInitState[T any](onInit func(ctx context.Context, state *T) (context.Context, error)) LoopableIteratorStateOption[T] {
	return func(li *LoopableIteratorState[T]) {
		li.onInit = onInit
	}
}

func NewLoopableIteratorState[T any](tasks []CancellableStateTask[T], state *T, opts ...LoopableIteratorStateOption[T]) *LoopableIteratorState[T] {
	li := &LoopableIteratorState[T]{
		tasks:       tasks,
		state:       state,
		index:       0,
		extraCancel: []context.CancelFunc{},
		stopCh:      make(chan struct{}),
		onError: func(err error) error {
			return err // Default: return the error as-is
		},
		shouldStop: func(err error) bool {
			return errors.Is(err, ErrLoopCritical)
		},
	}
	for _, opt := range opts {
		opt(li)
	}
	return li
}

func (li *LoopableIteratorState[T]) Run(ctx context.Context) chan error {
	if !li.running.CompareAndSwap(false, true) {
		return nil // Already running
	}

	if li.ctx != nil && li.ctx == ctx {
		return nil // Same context, can't rerun
	}

	li.ctx, li.cancel = context.WithCancel(ctx)
	li.index = 0
	li.stopCh = make(chan struct{})

	errChan := make(chan error, 1) // Buffered channel to avoid goroutine leak

	if li.onInit != nil {
		newCtx, err := li.onInit(li.ctx, li.state)
		if err != nil {
			errChan <- errors.Join(ErrLoopCritical, err)
			return errChan
		}
		li.ctx = newCtx
	}

	li.wg.Add(1)
	go func() {
		defer li.cleanup(errChan)

		for {
			select {
			case <-li.ctx.Done():
				errChan <- li.ctx.Err()
				return
			case <-li.stopCh:
				return
			default:
				if err := li.runIteration(); err != nil {
					if handledErr := li.onError(err); handledErr != nil {
						errChan <- handledErr
						if li.shouldStop(handledErr) {
							return
						}
					}
				}
			}
		}
	}()

	return errChan
}

func (li *LoopableIteratorState[T]) cleanup(errChan chan error) {
	li.cancel()
	li.runExtraCancel()
	close(errChan)
	li.running.Store(false)
	li.wg.Done()
}

func (li *LoopableIteratorState[T]) Wait() {
	li.wg.Wait()
}

func (li *LoopableIteratorState[T]) runExtraCancel() {
	for _, cancel := range li.extraCancel {
		cancel()
	}
}

func (li *LoopableIteratorState[T]) checkError(err error) error {
	var critical error
	if li.onError != nil && err != nil {
		critical = li.onError(err)
	}

	if errors.Is(err, ErrLoopCritical) {
		return err
	} else if critical != nil {
		return critical
	}

	return nil
}

func (li *LoopableIteratorState[T]) runIteration() error {
	if li.beforeLoop != nil {
		if err := li.checkError(li.beforeLoop(li.ctx, li.state)); err != nil {
			return err
		}
	}

	if err := li.checkError(li.runTasks()); err != nil {
		return err
	}

	if li.afterLoop != nil {
		if err := li.checkError(li.afterLoop(li.ctx, li.state)); err != nil {
			return err
		}
	}

	li.resetIndex()
	return nil
}

func (li *LoopableIteratorState[T]) runTasks() error {
	for li.index < len(li.tasks) {
		if err := li.tasks[li.index](li.ctx, li.state); err != nil {
			return err
		}
		li.incrementIndex()
	}
	return nil
}

func (li *LoopableIteratorState[T]) Stop() {
	close(li.stopCh)
}

func (li *LoopableIteratorState[T]) Cancel() {
	if li.cancel != nil {
		li.cancel()
	}
}

func (li *LoopableIteratorState[T]) resetIndex() {
	li.mu.Lock()
	defer li.mu.Unlock()
	li.index = 0
}

func (li *LoopableIteratorState[T]) incrementIndex() {
	li.mu.Lock()
	defer li.mu.Unlock()
	li.index++
}
