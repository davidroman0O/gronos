package gronos

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
)

var ErrLoopCritical = errors.New("critical error: stopping the loop")

type CancellableTask func(ctx context.Context) error

type LoopableIterator struct {
	tasks       []CancellableTask
	index       int
	mu          sync.Mutex
	onError     func(error) error
	shouldStop  func(error) bool
	beforeLoop  func() error
	afterLoop   func() error
	extraCancel []context.CancelFunc
	running     atomic.Bool
	stopCh      chan struct{}
	ctx         context.Context
	cancel      context.CancelFunc
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

func WithBeforeLoop(beforeLoop func() error) LoopableIteratorOption {
	return func(li *LoopableIterator) {
		li.beforeLoop = beforeLoop
	}
}

func WithAfterLoop(afterLoop func() error) LoopableIteratorOption {
	return func(li *LoopableIterator) {
		li.afterLoop = afterLoop
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

	errChan := make(chan error)

	go func() {
		defer close(errChan)
		defer li.running.Store(false)
		defer li.cancel()

		for {
			select {
			case <-li.ctx.Done():
				fmt.Println("Context done")
				for _, cancel := range li.extraCancel {
					cancel()
				}
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

func (li *LoopableIterator) runIteration() error {
	if li.beforeLoop != nil {
		if err := li.beforeLoop(); err != nil {
			return err
		}
	}

	if err := li.runTasks(); err != nil {
		return err
	}

	if li.afterLoop != nil {
		if err := li.afterLoop(); err != nil {
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
