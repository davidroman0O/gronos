package gronos

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

func TestLoopableIterator(t *testing.T) {
	t.Run("Basic functionality", func(t *testing.T) {
		var counter int32
		tasks := []CancellableTask{
			func(ctx context.Context) error {
				atomic.AddInt32(&counter, 1)
				return nil
			},
			func(ctx context.Context) error {
				atomic.AddInt32(&counter, 2)
				return nil
			},
		}

		li := NewLoopableIterator(tasks)
		ctx, cancel := context.WithCancel(context.Background())
		errChan := li.Run(ctx)

		time.Sleep(100 * time.Millisecond)
		cancel()

		for err := range errChan {
			if err != context.Canceled {
				t.Errorf("Unexpected error: %v", err)
			}
		}

		if atomic.LoadInt32(&counter) <= 3 {
			t.Errorf("Expected counter to be greater than 3, got %d", counter)
		}
	})

	t.Run("Error handling", func(t *testing.T) {
		errExpected := errors.New("expected error")
		var errCount int32

		tasks := []CancellableTask{
			func(ctx context.Context) error {
				atomic.AddInt32(&errCount, 1)
				return errExpected
			},
		}

		li := NewLoopableIterator(tasks)
		ctx, cancel := context.WithCancel(context.Background())
		errChan := li.Run(ctx)

		// Wait for a short time to allow multiple errors to occur
		time.Sleep(100 * time.Millisecond)

		// Cancel the iterator
		cancel()

		// Collect all errors from the channel
		var receivedErrors []error
		for err := range errChan {
			receivedErrors = append(receivedErrors, err)
		}

		// Check if we received any errors
		if len(receivedErrors) == 0 {
			t.Error("Expected to receive errors, but got none")
		}

		// Check if all received errors are the expected error
		for _, err := range receivedErrors {
			if err != errExpected && err != context.Canceled {
				t.Errorf("Expected error %v or context.Canceled, got %v", errExpected, err)
			}
		}

		// Check if the error count is greater than 0
		if atomic.LoadInt32(&errCount) == 0 {
			t.Error("Expected error count to be greater than 0")
		}

		t.Logf("Received %d errors, error count: %d", len(receivedErrors), atomic.LoadInt32(&errCount))
	})

	t.Run("Critical error handling", func(t *testing.T) {
		ctx := context.Background()

		criticalErr := errors.New("critical error")
		tasks := []CancellableTask{
			func(ctx context.Context) error {
				return errors.Join(criticalErr, ErrLoopCritical)
			},
		}

		li := NewLoopableIterator(tasks)
		errChan := li.Run(ctx)

		err := <-errChan
		if !errors.Is(err, ErrLoopCritical) {
			t.Errorf("Expected ErrLoopCritical, got %v", err)
		}

		_, open := <-errChan
		if open {
			t.Error("Expected error channel to be closed after critical error")
		}
	})

	t.Run("Before and after hooks", func(t *testing.T) {
		var beforeCount, afterCount, taskCount int32

		li := NewLoopableIterator(
			[]CancellableTask{
				func(ctx context.Context) error {
					atomic.AddInt32(&taskCount, 1)
					return nil
				},
			},
			WithBeforeLoop(func(_ context.Context) error {
				atomic.AddInt32(&beforeCount, 1)
				return nil
			}),
			WithAfterLoop(func(_ context.Context) error {
				atomic.AddInt32(&afterCount, 1)
				return nil
			}),
		)

		ctx, cancel := context.WithCancel(context.Background())
		errChan := li.Run(ctx)

		time.Sleep(100 * time.Millisecond)
		cancel()

		for range errChan {
			// Drain the channel
		}

		before := atomic.LoadInt32(&beforeCount)
		after := atomic.LoadInt32(&afterCount)
		tasks := atomic.LoadInt32(&taskCount)

		if before == 0 || after == 0 || tasks == 0 {
			t.Error("Expected non-zero counts for before, after, and tasks")
		}

		if before != after {
			t.Errorf("Before (%d) and after (%d) counts don't match", before, after)
		}

		if tasks != before {
			t.Errorf("Task count (%d) doesn't match before count (%d)", tasks, before)
		}

		t.Logf("Iterations completed: before=%d, after=%d, tasks=%d", before, after, tasks)
	})
}
