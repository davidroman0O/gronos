package gronos

import (
	"context"
	"errors"

	"github.com/charmbracelet/log"
)

// IteratorOption is a function type for configuring the Iterator middleware.
type IteratorOption func(*iteratorConfig)

type iteratorConfig struct {
	loopOpts []LoopableIteratorOption
}

// WithLoopableIteratorOptions adds LoopableIteratorOptions to the Iterator middleware.
func WithLoopableIteratorOptions(opts ...LoopableIteratorOption) IteratorOption {
	return func(c *iteratorConfig) {
		c.loopOpts = append(c.loopOpts, opts...)
	}
}

// Iterator creates a RuntimeApplication that uses a LoopableIterator to execute tasks.
func Iterator(tasks []CancellableTask, opts ...IteratorOption) RuntimeApplication {
	config := &iteratorConfig{}
	for _, opt := range opts {
		opt(config)
	}
	log.Debug("[Iterator] Creating iterator")
	return func(ctx context.Context, shutdown <-chan struct{}) error {
		li := NewLoopableIterator(tasks, config.loopOpts...)
		errChan := li.Run(ctx)
		log.Debug("[Iterator] Starting iterator")

		defer li.Cancel()
		var finalErr error
		select {
		case <-ctx.Done():
			log.Debug("[Iterator] Context done")
			li.Cancel()
			finalErr = ctx.Err()
		case <-shutdown:
			log.Debug("[Iterator] Shutdown")
			li.Stop()
			li.Cancel()
		case err, ok := <-errChan:
			if !ok {
				return nil
			}
			if errors.Is(err, ErrLoopCritical) {
				li.Cancel()
				finalErr = err
			}
			log.Debug("[Iterator] Error", finalErr)
		}

		li.Wait()

		return finalErr
	}
}
