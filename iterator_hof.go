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
func Iterator(iterCtx context.Context, tasks []CancellableTask, opts ...IteratorOption) RuntimeApplication {
	config := &iteratorConfig{}
	for _, opt := range opts {
		opt(config)
	}
	log.Debug("[Iterator] Creating iterator")
	return func(appCtx context.Context, shutdown <-chan struct{}) error {

		li := NewLoopableIterator(tasks, config.loopOpts...) // only the loopable iterator will define when to stop
		errChan := li.Run(iterCtx)
		log.Debug("[Iterator] Starting iterator")

		var finalErr error
		select {
		case <-iterCtx.Done():
			log.Debug("[Iterator] Context done")
			li.Cancel()
			finalErr = iterCtx.Err()
		case <-appCtx.Done():
			log.Debug("[Iterator] App context done")
			li.Cancel()
			finalErr = appCtx.Err()
		case <-shutdown:
			log.Debug("[Iterator] Shutdown")
			li.Cancel()
		case err, ok := <-errChan:
			if !ok {
				// Error channel closed, iterator has finished
				return nil
			}
			if errors.Is(err, ErrLoopCritical) {
				li.Cancel()
				finalErr = err
			}
			log.Debug("[Iterator] Error", finalErr)
		}

		li.Wait() // Wait for the iterator to finish

		return finalErr
	}
}
