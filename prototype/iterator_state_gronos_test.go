package gronos

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"
	"testing"
	"time"
)

type testStateIteratorGronos struct {
	Counter int32
}

func TestIteratorStateWithGronos(t *testing.T) {
	t.Run("Basic functionality", func(t *testing.T) {
		state := &testStateIteratorGronos{}
		tasks := []CancellableStateTask[testStateIteratorGronos]{
			func(ctx context.Context, s *testStateIteratorGronos) error {
				atomic.AddInt32(&s.Counter, 1)
				time.Sleep(10 * time.Millisecond)
				return nil
			},
		}

		iterApp := IteratorState(tasks, WithInitialState(state))

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		g, errChan := New[string](ctx, map[string]LifecyleFunc{
			"iterator": iterApp,
		})

		time.Sleep(100 * time.Millisecond)
		g.Shutdown()
		g.Wait()

		select {
		case err := <-errChan:
			if err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
		default:
		}

		if atomic.LoadInt32(&state.Counter) == 0 {
			t.Error("Expected at least one iteration")
		}
	})

	t.Run("Context cancellation", func(t *testing.T) {
		state := &testStateIteratorGronos{}
		tasks := []CancellableStateTask[testStateIteratorGronos]{
			func(ctx context.Context, s *testStateIteratorGronos) error {
				atomic.AddInt32(&s.Counter, 1)
				time.Sleep(10 * time.Millisecond)
				return nil
			},
		}

		iterApp := IteratorState(tasks, WithInitialState(state))

		ctx, cancel := context.WithCancel(context.Background())
		g, errChan := New[string](ctx, map[string]LifecyleFunc{
			"iterator": iterApp,
		})

		time.Sleep(50 * time.Millisecond)
		cancel()

		g.Wait()

		select {
		case err := <-errChan:
			if err != context.Canceled {
				t.Errorf("Expected context.Canceled, got: %v", err)
			}
		case <-time.After(1 * time.Second):
			t.Error("Timeout waiting for error")
		}

		if atomic.LoadInt32(&state.Counter) == 0 {
			t.Error("Expected at least one iteration before cancellation")
		}
	})

	t.Run("Error handling", func(t *testing.T) {
		expectedError := errors.New("expected error")
		state := &testStateIteratorGronos{}

		tasks := []CancellableStateTask[testStateIteratorGronos]{
			func(ctx context.Context, s *testStateIteratorGronos) error {
				atomic.AddInt32(&s.Counter, 1)
				return expectedError
			},
		}

		iterApp := IteratorState(tasks, WithInitialState(state),
			WithLoopableIteratorStateOptions(
				WithOnErrorState[testStateIteratorGronos](func(err error) error {
					return errors.Join(err, ErrLoopCritical)
				}),
			),
		)

		ctx := context.Background()
		g, errChan := New[string](ctx, map[string]LifecyleFunc{
			"iterator": iterApp,
		})

		select {
		case err := <-errChan:
			fmt.Println("unit test error:", err)
			if !errors.Is(err, ErrLoopCritical) {
				t.Errorf("Expected ErrLoopCritical, got: %v", err)
			}
			if !errors.Is(err, expectedError) {
				t.Errorf("Expected error to contain %v, got: %v", expectedError, err)
			}
		case <-time.After(1 * time.Second):
			t.Error("Timeout waiting for error")
		}

		g.Shutdown()
		g.Wait()

		if atomic.LoadInt32(&state.Counter) == 0 {
			t.Error("Expected error to occur at least once")
		}
	})

	t.Run("Graceful shutdown", func(t *testing.T) {
		state := &testStateIteratorGronos{}
		tasks := []CancellableStateTask[testStateIteratorGronos]{
			func(ctx context.Context, s *testStateIteratorGronos) error {
				atomic.AddInt32(&s.Counter, 1)
				time.Sleep(10 * time.Millisecond)
				return nil
			},
		}

		iterApp := IteratorState(tasks, WithInitialState(state))

		ctx := context.Background()
		g, errChan := New[string](ctx, map[string]LifecyleFunc{
			"iterator": iterApp,
		})

		time.Sleep(50 * time.Millisecond)
		g.Shutdown()
		g.Wait()

		select {
		case err := <-errChan:
			if err != nil {
				t.Errorf("Expected nil error on shutdown, got: %v", err)
			}
		case <-time.After(1 * time.Second):
			t.Error("Timeout waiting for graceful shutdown")
		}

		if atomic.LoadInt32(&state.Counter) == 0 {
			t.Error("Expected at least one iteration before shutdown")
		}
	})

	t.Run("Multiple iterators", func(t *testing.T) {
		state1 := &testStateIteratorGronos{}
		state2 := &testStateIteratorGronos{}

		tasks1 := []CancellableStateTask[testStateIteratorGronos]{
			func(ctx context.Context, s *testStateIteratorGronos) error {
				atomic.AddInt32(&s.Counter, 1)
				time.Sleep(10 * time.Millisecond)
				return nil
			},
		}

		tasks2 := []CancellableStateTask[testStateIteratorGronos]{
			func(ctx context.Context, s *testStateIteratorGronos) error {
				atomic.AddInt32(&s.Counter, 2)
				time.Sleep(15 * time.Millisecond)
				return nil
			},
		}

		iterApp1 := IteratorState(tasks1, WithInitialState(state1))
		iterApp2 := IteratorState(tasks2, WithInitialState(state2))

		ctx := context.Background()
		g, errChan := New[string](ctx, map[string]LifecyleFunc{
			"iterator1": iterApp1,
			"iterator2": iterApp2,
		})

		time.Sleep(100 * time.Millisecond)
		g.Shutdown()
		g.Wait()

		select {
		case err := <-errChan:
			if err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
		default:
		}

		if atomic.LoadInt32(&state1.Counter) == 0 {
			t.Error("Expected at least one iteration for iterator1")
		}

		if atomic.LoadInt32(&state2.Counter) == 0 {
			t.Error("Expected at least one iteration for iterator2")
		}
	})

	t.Run("BeforeLoop and AfterLoop hooks", func(t *testing.T) {
		state := &testStateIteratorGronos{}
		var beforeLoopCount, afterLoopCount int32

		tasks := []CancellableStateTask[testStateIteratorGronos]{
			func(ctx context.Context, s *testStateIteratorGronos) error {
				atomic.AddInt32(&s.Counter, 1)
				time.Sleep(10 * time.Millisecond)
				return nil
			},
		}

		iterApp := IteratorState(
			tasks,
			WithInitialState(state),
			WithLoopableIteratorStateOptions(
				WithBeforeLoopState[testStateIteratorGronos](func(ctx context.Context, s *testStateIteratorGronos) error {
					atomic.AddInt32(&beforeLoopCount, 1)
					return nil
				}),
				WithAfterLoopState[testStateIteratorGronos](func(ctx context.Context, s *testStateIteratorGronos) error {
					atomic.AddInt32(&afterLoopCount, 1)
					return nil
				}),
			),
		)

		ctx := context.Background()
		g, errChan := New[string](ctx, map[string]LifecyleFunc{
			"iterator": iterApp,
		})

		time.Sleep(100 * time.Millisecond)
		g.Shutdown()
		g.Wait()

		select {
		case err := <-errChan:
			if err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
		default:
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

	t.Run("OnInit hook", func(t *testing.T) {
		state := &testStateIteratorGronos{}
		var initCalled bool

		tasks := []CancellableStateTask[testStateIteratorGronos]{
			func(ctx context.Context, s *testStateIteratorGronos) error {
				atomic.AddInt32(&s.Counter, 1)
				return nil
			},
		}

		iterApp := IteratorState(
			tasks,
			WithInitialState(state),
			WithLoopableIteratorStateOptions(
				WithOnInitState[testStateIteratorGronos](func(ctx context.Context, s *testStateIteratorGronos) (context.Context, error) {
					initCalled = true
					return ctx, nil
				}),
			),
		)

		ctx := context.Background()
		g, errChan := New[string](ctx, map[string]LifecyleFunc{
			"iterator": iterApp,
		})

		time.Sleep(50 * time.Millisecond)
		g.Shutdown()
		g.Wait()

		select {
		case err := <-errChan:
			if err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
		default:
		}

		if !initCalled {
			t.Error("Expected OnInit to be called")
		}

		if atomic.LoadInt32(&state.Counter) == 0 {
			t.Error("Expected at least one iteration")
		}
	})
}
