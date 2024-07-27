package gronos

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

func TestIteratorMiddleware(t *testing.T) {
	t.Run("Basic functionality", func(t *testing.T) {
		appCtx, appCancel := context.WithCancel(context.Background())
		defer appCancel()

		iterCtx, iterCancel := context.WithCancel(context.Background())
		defer iterCancel()

		var counter int32
		tasks := []CancellableTask{
			func(ctx context.Context) error {
				atomic.AddInt32(&counter, 1)
				time.Sleep(10 * time.Millisecond)
				return nil
			},
		}

		iterApp := Iterator(iterCtx, tasks)

		shutdown := make(chan struct{})
		errChan := make(chan error, 1)

		go func() {
			errChan <- iterApp(appCtx, shutdown)
		}()

		time.Sleep(100 * time.Millisecond)
		close(shutdown)

		err := <-errChan
		if err != nil {
			t.Errorf("Unexpected error: %v", err)
		}

		if atomic.LoadInt32(&counter) == 0 {
			t.Error("Expected at least one iteration")
		}
	})

	t.Run("Iterator context cancellation", func(t *testing.T) {
		appCtx := context.Background()
		iterCtx, iterCancel := context.WithCancel(context.Background())

		var counter int32
		tasks := []CancellableTask{
			func(ctx context.Context) error {
				atomic.AddInt32(&counter, 1)
				time.Sleep(10 * time.Millisecond)
				return nil
			},
		}

		iterApp := Iterator(iterCtx, tasks)

		shutdown := make(chan struct{})
		errChan := make(chan error, 1)

		go func() {
			errChan <- iterApp(appCtx, shutdown)
		}()

		time.Sleep(50 * time.Millisecond)
		iterCancel()

		err := <-errChan
		if err != context.Canceled {
			t.Errorf("Expected context.Canceled, got: %v", err)
		}

		if atomic.LoadInt32(&counter) == 0 {
			t.Error("Expected at least one iteration before cancellation")
		}
	})

	t.Run("Application context cancellation", func(t *testing.T) {
		appCtx, appCancel := context.WithCancel(context.Background())
		iterCtx := context.Background()

		var counter int32
		tasks := []CancellableTask{
			func(ctx context.Context) error {
				atomic.AddInt32(&counter, 1)
				time.Sleep(10 * time.Millisecond)
				return nil
			},
		}

		iterApp := Iterator(iterCtx, tasks)

		shutdown := make(chan struct{})
		errChan := make(chan error, 1)

		go func() {
			errChan <- iterApp(appCtx, shutdown)
		}()

		time.Sleep(50 * time.Millisecond)
		appCancel()

		err := <-errChan
		if err != context.Canceled {
			t.Errorf("Expected context.Canceled, got: %v", err)
		}

		if atomic.LoadInt32(&counter) == 0 {
			t.Error("Expected at least one iteration before cancellation")
		}
	})

	t.Run("Error handling", func(t *testing.T) {
		appCtx := context.Background()
		iterCtx := context.Background()

		expectedError := errors.New("expected error")
		var errorCount int32

		tasks := []CancellableTask{
			func(ctx context.Context) error {
				atomic.AddInt32(&errorCount, 1)
				return expectedError
			},
		}

		iterApp := Iterator(iterCtx, tasks, WithLoopableIteratorOptions(
			WithOnError(func(err error) error {
				return errors.Join(err, ErrLoopCritical)
			}),
		))

		shutdown := make(chan struct{})
		errChan := make(chan error, 1)

		go func() {
			errChan <- iterApp(appCtx, shutdown)
		}()

		select {
		case err := <-errChan:
			if !errors.Is(err, ErrLoopCritical) {
				t.Errorf("Expected ErrLoopCritical, got: %v", err)
			}
			if !errors.Is(err, expectedError) {
				t.Errorf("Expected error to contain %v, got: %v", expectedError, err)
			}
		case <-time.After(1 * time.Second):
			t.Error("Timeout waiting for error")
		}

		if atomic.LoadInt32(&errorCount) == 0 {
			t.Error("Expected error to occur at least once")
		}
	})

	t.Run("Shutdown signal", func(t *testing.T) {
		appCtx := context.Background()
		iterCtx := context.Background()

		var counter int32
		tasks := []CancellableTask{
			func(ctx context.Context) error {
				atomic.AddInt32(&counter, 1)
				time.Sleep(10 * time.Millisecond)
				return nil
			},
		}

		iterApp := Iterator(iterCtx, tasks)

		shutdown := make(chan struct{})
		errChan := make(chan error, 1)

		go func() {
			errChan <- iterApp(appCtx, shutdown)
		}()

		time.Sleep(50 * time.Millisecond)
		close(shutdown)

		err := <-errChan
		if err != nil {
			t.Errorf("Expected nil error on shutdown, got: %v", err)
		}

		if atomic.LoadInt32(&counter) == 0 {
			t.Error("Expected at least one iteration before shutdown")
		}
	})

	t.Run("Critical error handling", func(t *testing.T) {
		appCtx := context.Background()
		iterCtx := context.Background()

		criticalErr := errors.New("critical error")
		tasks := []CancellableTask{
			func(ctx context.Context) error {
				return errors.Join(criticalErr, ErrLoopCritical)
			},
		}

		iterApp := Iterator(iterCtx, tasks)

		shutdown := make(chan struct{})
		errChan := make(chan error, 1)

		go func() {
			errChan <- iterApp(appCtx, shutdown)
		}()

		err := <-errChan
		if !errors.Is(err, ErrLoopCritical) {
			t.Errorf("Expected ErrLoopCritical, got: %v", err)
		}
	})
}
