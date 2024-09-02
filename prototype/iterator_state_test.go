package gronos

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

type testState struct {
	Counter int32
}

func TestIteratorStateHOF(t *testing.T) {
	t.Run("Basic functionality", func(t *testing.T) {
		state := &testState{}
		tasks := []CancellableStateTask[testState]{
			func(ctx context.Context, s *testState) error {
				atomic.AddInt32(&s.Counter, 1)
				time.Sleep(10 * time.Millisecond)
				return nil
			},
		}

		iterApp := IteratorState(tasks, WithInitialState(state))

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		shutdown := make(chan struct{})
		errChan := make(chan error, 1)

		go func() {
			errChan <- iterApp(ctx, shutdown)
		}()

		time.Sleep(100 * time.Millisecond)
		close(shutdown)

		err := <-errChan
		if err != nil {
			t.Errorf("Unexpected error: %v", err)
		}

		if atomic.LoadInt32(&state.Counter) == 0 {
			t.Error("Expected at least one iteration")
		}
	})

	t.Run("Context cancellation", func(t *testing.T) {
		state := &testState{}
		tasks := []CancellableStateTask[testState]{
			func(ctx context.Context, s *testState) error {
				atomic.AddInt32(&s.Counter, 1)
				time.Sleep(10 * time.Millisecond)
				return nil
			},
		}

		iterApp := IteratorState(tasks, WithInitialState(state))

		ctx, cancel := context.WithCancel(context.Background())
		shutdown := make(chan struct{})
		errChan := make(chan error, 1)

		go func() {
			errChan <- iterApp(ctx, shutdown)
		}()

		time.Sleep(50 * time.Millisecond)
		cancel()

		err := <-errChan
		if err != context.Canceled {
			t.Errorf("Expected context.Canceled, got: %v", err)
		}

		if atomic.LoadInt32(&state.Counter) == 0 {
			t.Error("Expected at least one iteration before cancellation")
		}
	})

	t.Run("Error handling", func(t *testing.T) {
		expectedError := errors.New("expected error")
		state := &testState{}

		tasks := []CancellableStateTask[testState]{
			func(ctx context.Context, s *testState) error {
				atomic.AddInt32(&s.Counter, 1)
				return expectedError
			},
		}

		iterApp := IteratorState(tasks, WithInitialState(state),
			WithLoopableIteratorStateOptions(
				WithOnErrorState[testState](func(err error) error {
					return errors.Join(err, ErrLoopCritical)
				}),
			),
		)

		ctx := context.Background()
		shutdown := make(chan struct{})
		errChan := make(chan error, 1)

		go func() {
			errChan <- iterApp(ctx, shutdown)
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

		if atomic.LoadInt32(&state.Counter) == 0 {
			t.Error("Expected error to occur at least once")
		}
	})

	t.Run("Shutdown signal", func(t *testing.T) {
		state := &testState{}
		tasks := []CancellableStateTask[testState]{
			func(ctx context.Context, s *testState) error {
				atomic.AddInt32(&s.Counter, 1)
				time.Sleep(10 * time.Millisecond)
				return nil
			},
		}

		iterApp := IteratorState(tasks, WithInitialState(state))

		ctx := context.Background()
		shutdown := make(chan struct{})
		errChan := make(chan error, 1)

		go func() {
			errChan <- iterApp(ctx, shutdown)
		}()

		time.Sleep(50 * time.Millisecond)
		close(shutdown)

		err := <-errChan
		if err != nil {
			t.Errorf("Expected nil error on shutdown, got: %v", err)
		}

		if atomic.LoadInt32(&state.Counter) == 0 {
			t.Error("Expected at least one iteration before shutdown")
		}
	})

	t.Run("Critical error handling", func(t *testing.T) {
		criticalErr := errors.New("critical error")
		state := &testState{}
		tasks := []CancellableStateTask[testState]{
			func(ctx context.Context, s *testState) error {
				atomic.AddInt32(&s.Counter, 1)
				return errors.Join(criticalErr, ErrLoopCritical)
			},
		}

		iterApp := IteratorState(tasks, WithInitialState(state))

		ctx := context.Background()
		shutdown := make(chan struct{})
		errChan := make(chan error, 1)

		go func() {
			errChan <- iterApp(ctx, shutdown)
		}()

		err := <-errChan
		if !errors.Is(err, ErrLoopCritical) {
			t.Errorf("Expected ErrLoopCritical, got: %v", err)
		}

		if atomic.LoadInt32(&state.Counter) != 1 {
			t.Error("Expected exactly one iteration before critical error")
		}
	})

	t.Run("Error handling and graceful shutdown", func(t *testing.T) {
		expectedError := errors.New("expected error")
		state := &testState{}
		var cleanupExecuted int32

		cleanup := context.CancelFunc(func() {
			atomic.AddInt32(&cleanupExecuted, 1)
		})

		tasks := []CancellableStateTask[testState]{
			func(ctx context.Context, s *testState) error {
				atomic.AddInt32(&s.Counter, 1)
				return expectedError
			},
		}

		iterApp := IteratorState(tasks, WithInitialState(state),
			WithLoopableIteratorStateOptions(
				WithOnErrorState[testState](func(err error) error {
					return errors.Join(ErrLoopCritical, err)
				}),
				WithExtraCancelState[testState](cleanup),
			),
		)

		ctx, cancel := context.WithCancel(context.Background())
		g, errChan := New[string](ctx, map[string]LifecyleFunc{
			"iterator": iterApp,
		})

		// Wait a bit to ensure the task has a chance to execute
		time.Sleep(50 * time.Millisecond)

		// Cancel the context to trigger shutdown
		cancel()

		// Wait for Gronos to finish
		g.Wait()

		select {
		case err := <-errChan:
			if !errors.Is(err, ErrLoopCritical) || !errors.Is(err, expectedError) {
				t.Errorf("Expected ErrLoopCritical and %v, got: %v", expectedError, err)
			}
		case <-time.After(1 * time.Second):
			t.Error("Timeout waiting for error")
		}

		if atomic.LoadInt32(&state.Counter) == 0 {
			t.Error("Expected error to occur at least once")
		}

		if atomic.LoadInt32(&cleanupExecuted) == 0 {
			t.Error("Expected cleanup (extraCancel) to be executed")
		}
	})
	t.Run("BeforeLoop and AfterLoop hooks", func(t *testing.T) {
		state := &testState{}
		var beforeLoopCount, afterLoopCount int32

		tasks := []CancellableStateTask[testState]{
			func(ctx context.Context, s *testState) error {
				atomic.AddInt32(&s.Counter, 1)
				time.Sleep(10 * time.Millisecond)
				return nil
			},
		}

		iterApp := IteratorState(
			tasks,
			WithInitialState(state),
			WithLoopableIteratorStateOptions(
				WithBeforeLoopState[testState](func(ctx context.Context, s *testState) error {
					atomic.AddInt32(&beforeLoopCount, 1)
					return nil
				}),
				WithAfterLoopState[testState](func(ctx context.Context, s *testState) error {
					atomic.AddInt32(&afterLoopCount, 1)
					return nil
				}),
			),
		)

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		shutdown := make(chan struct{})
		errChan := make(chan error, 1)

		go func() {
			errChan <- iterApp(ctx, shutdown)
		}()

		time.Sleep(100 * time.Millisecond)
		close(shutdown)

		err := <-errChan
		if err != nil {
			t.Errorf("Unexpected error: %v", err)
		}

		if atomic.LoadInt32(&beforeLoopCount) == 0 {
			t.Error("Expected BeforeLoop to be executed at least once")
		}

		if atomic.LoadInt32(&afterLoopCount) == 0 {
			t.Error("Expected AfterLoop to be executed at least once")
		}

		if atomic.LoadInt32(&state.Counter) == 0 {
			t.Error("Expected at least one iteration")
		}

		if atomic.LoadInt32(&beforeLoopCount) != atomic.LoadInt32(&afterLoopCount) {
			t.Error("Expected BeforeLoop and AfterLoop to be executed the same number of times")
		}
	})
}
