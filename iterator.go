package gronos

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
)

var ErrLoopCritical = errors.New("critical error")

type CancellableTask func(ctx context.Context) error

type LoopableIterator struct {
	tasks       []CancellableTask
	index       int
	mu          sync.Mutex
	onInit      func(ctx context.Context) (context.Context, error)
	onError     func(error) error
	shouldStop  func(error) bool
	beforeLoop  func(ctx context.Context) error
	afterLoop   func(ctx context.Context) error
	extraCancel []context.CancelFunc
	running     atomic.Bool
	stopCh      chan struct{}
	ctx         context.Context
	cancel      context.CancelFunc
	wg          sync.WaitGroup
}

type LoopableIteratorOption func(*LoopableIterator)

func WithExtraCancel(cancels ...context.CancelFunc) LoopableIteratorOption {
	return func(li *LoopableIterator) {
		li.extraCancel = append(li.extraCancel, cancels...)
	}
}

func WithOnError(handler func(error) error) LoopableIteratorOption {
	return func(li *LoopableIterator) {
		li.onError = handler
	}
}

func WithShouldStop(shouldStop func(error) bool) LoopableIteratorOption {
	return func(li *LoopableIterator) {
		li.shouldStop = shouldStop
	}
}

func WithBeforeLoop(beforeLoop func(ctx context.Context) error) LoopableIteratorOption {
	return func(li *LoopableIterator) {
		li.beforeLoop = beforeLoop
	}
}

func WithAfterLoop(afterLoop func(ctx context.Context) error) LoopableIteratorOption {
	return func(li *LoopableIterator) {
		li.afterLoop = afterLoop
	}
}

func WithOnInit(onInit func(ctx context.Context) (context.Context, error)) LoopableIteratorOption {
	return func(li *LoopableIterator) {
		li.onInit = onInit
	}
}

func NewLoopableIterator(tasks []CancellableTask, opts ...LoopableIteratorOption) *LoopableIterator {
	li := &LoopableIterator{
		tasks:       tasks,
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

func (li *LoopableIterator) Run(ctx context.Context) chan error {
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
		ctx, err := li.onInit(context.Background())
		if err != nil {
			errChan <- errors.Join(ErrLoopCritical, err)
			return errChan
		}
		li.ctx = ctx
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

func (li *LoopableIterator) cleanup(errChan chan error) {
	li.cancel()
	li.runExtraCancel()
	close(errChan)
	li.running.Store(false)
	li.wg.Done()
}

func (li *LoopableIterator) Wait() {
	li.wg.Wait()
}

func (li *LoopableIterator) runExtraCancel() {
	for _, cancel := range li.extraCancel {
		cancel()
	}
}

func (li *LoopableIterator) checkError(err error) error {
	var critical error
	// both conditions are important, we shouldn't trigger onError if the `err` is nil
	if li.onError != nil && err != nil {
		critical = li.onError(err)
	}

	// Only ErrLoopCritical will stop the loop
	if errors.Is(err, ErrLoopCritical) {
		return err
	} else if critical != nil {
		return critical
	}

	return nil
}

func (li *LoopableIterator) runIteration() error {
	if li.beforeLoop != nil {
		if err := li.checkError(li.beforeLoop(li.ctx)); err != nil {
			return err
		}
	}

	if err := li.checkError(li.runTasks()); err != nil {
		return err
	}

	if li.afterLoop != nil {
		if err := li.checkError(li.afterLoop(li.ctx)); err != nil {
			return err
		}
	}

	li.resetIndex()
	return nil
}

func (li *LoopableIterator) runTasks() error {
	for li.index < len(li.tasks) {
		if err := li.tasks[li.index](li.ctx); err != nil {
			return err
		}
		li.incrementIndex()
	}
	return nil
}

func (li *LoopableIterator) Stop() {
	close(li.stopCh)
}

func (li *LoopableIterator) Cancel() {
	if li.cancel != nil {
		li.cancel()
	}
}

func (li *LoopableIterator) resetIndex() {
	li.mu.Lock()
	defer li.mu.Unlock()
	li.index = 0
}

func (li *LoopableIterator) incrementIndex() {
	li.mu.Lock()
	defer li.mu.Unlock()
	li.index++
}
