package gronos

import (
	"context"
	"errors"
)

// ErrorListener is a function type for listening to errors from the LoopableIterator.
type ErrorListener func(error)

// IteratorOption is a function type for configuring the Iterator middleware.
type IteratorOption func(*iteratorConfig)

type iteratorConfig struct {
	errorListener ErrorListener
	loopOpts      []LoopableIteratorOption
}

// WithErrorListener sets an error listener for the Iterator middleware.
func WithErrorListener(listener ErrorListener) IteratorOption {
	return func(c *iteratorConfig) {
		c.errorListener = listener
	}
}

// WithLoopableIteratorOptions adds LoopableIteratorOptions to the Iterator middleware.
func WithLoopableIteratorOptions(opts ...LoopableIteratorOption) IteratorOption {
	return func(c *iteratorConfig) {
		c.loopOpts = append(c.loopOpts, opts...)
	}
}

// Iterator creates a RuntimeApplication that uses a LoopableIterator to execute tasks.
func Iterator(iterCtx context.Context, tasks []CancellableTask, opts ...IteratorOption) RuntimeApplication {
	config := &iteratorConfig{}
	for _, opt := range opts {
		opt(config)
	}

	return func(appCtx context.Context, shutdown <-chan struct{}) error {
		// Create a new context that's cancelled when either the iterator context
		// or the application context is cancelled
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		go func() {
			select {
			case <-iterCtx.Done():
				cancel()
			case <-appCtx.Done():
				cancel()
			case <-shutdown:
				cancel()
			}
		}()

		li := NewLoopableIterator(ctx, tasks, config.loopOpts...)
		errChan := li.Run()

		for {
			select {
			case <-iterCtx.Done():
				li.Cancel()
				return iterCtx.Err()
			case <-appCtx.Done():
				li.Cancel()
				return appCtx.Err()
			case <-shutdown:
				li.Cancel()
				return nil
			case err, ok := <-errChan:
				if !ok {
					// Error channel closed, iterator has finished
					return nil
				}
				if config.errorListener != nil {
					config.errorListener(err)
				}
				if errors.Is(err, ErrLoopCritical) {
					li.Cancel()
					return err
				}
				// Non-critical errors are ignored, and the loop continues
			}
		}
	}
}
