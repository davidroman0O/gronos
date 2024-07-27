package gronos

import (
	"context"
	"errors"
	"sync"
)

var ErrLoopCritical = errors.New("critical error: stopping the loop")

type CancellableTask func(ctx context.Context) error

type LoopableIterator struct {
	ctx         context.Context
	cancel      context.CancelFunc
	extraCancel []context.CancelFunc
	tasks       []CancellableTask
	index       int
	done        chan struct{}
	mu          sync.Mutex
	onError     func(error)
	shouldStop  func(error) bool
	beforeLoop  func() error
	afterLoop   func() error
}

type LoopableIteratorOption func(*LoopableIterator)

func WithExtraCancel(cancels ...context.CancelFunc) LoopableIteratorOption {
	return func(li *LoopableIterator) {
		li.extraCancel = append(li.extraCancel, cancels...)
	}
}

func WithOnError(handler func(error)) LoopableIteratorOption {
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

func NewLoopableIterator(ctx context.Context, tasks []CancellableTask, opts ...LoopableIteratorOption) *LoopableIterator {
	ctx, cancel := context.WithCancel(ctx)
	li := &LoopableIterator{
		ctx:         ctx,
		cancel:      cancel,
		tasks:       tasks,
		index:       0,
		done:        make(chan struct{}),
		extraCancel: []context.CancelFunc{},
		onError: func(err error) {
			// Default error handler
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

func (li *LoopableIterator) Run() chan error {
	errChan := make(chan error)

	go func() {
		defer close(li.done)
		defer close(errChan)

		for {
			select {
			case <-li.ctx.Done():
				for _, cancel := range li.extraCancel {
					cancel()
				}
				errChan <- li.ctx.Err()
				return
			default:
				if err := li.runIteration(); err != nil {
					errChan <- err
					if li.shouldStop(err) {
						return
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
			li.onError(err)
			return err
		}
	}

	if err := li.runTasks(); err != nil {
		li.onError(err)
		return err
	}

	if li.afterLoop != nil {
		if err := li.afterLoop(); err != nil {
			li.onError(err)
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

func (li *LoopableIterator) Wait() {
	<-li.done
}

func (li *LoopableIterator) Cancel() {
	li.cancel()
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
