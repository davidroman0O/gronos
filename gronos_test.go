package gronos

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestGronos(t *testing.T) {
	t.Run("Basic functionality", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		appStarted := make(chan struct{})
		appFinished := make(chan struct{})

		g := New[string](ctx, map[string]RuntimeApplication{
			"test-app": func(ctx context.Context, shutdown <-chan struct{}) error {
				close(appStarted)
				select {
				case <-ctx.Done():
					close(appFinished)
					return ctx.Err()
				case <-shutdown:
					close(appFinished)
					return nil
				}
			},
		})

		errors := g.Start()

		select {
		case <-appStarted:
			// App started successfully
		case <-time.After(time.Second):
			t.Fatal("Timeout waiting for app to start")
		}

		g.Shutdown()

		select {
		case <-appFinished:
			// App finished successfully
		case <-time.After(time.Second):
			t.Fatal("Timeout waiting for app to finish")
		}

		g.Wait()

		select {
		case err, ok := <-errors:
			if ok {
				if err != nil && err != context.Canceled {
					t.Fatalf("Unexpected error: %v", err)
				}
			}
		case <-time.After(time.Second):
			t.Fatal("Timeout waiting for error channel to close")
		}
	})

	t.Run("Multiple applications", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		appCount := 3
		appStarted := make([]chan struct{}, appCount)
		appFinished := make([]chan struct{}, appCount)

		apps := make(map[string]RuntimeApplication)
		for i := 0; i < appCount; i++ {
			appStarted[i] = make(chan struct{})
			appFinished[i] = make(chan struct{})
			index := i
			apps[fmt.Sprintf("app-%d", i)] = func(ctx context.Context, shutdown <-chan struct{}) error {
				close(appStarted[index])
				<-ctx.Done()
				close(appFinished[index])
				return ctx.Err()
			}
		}

		g := New[string](ctx, apps)
		errors := g.Start()

		for i := 0; i < appCount; i++ {
			select {
			case <-appStarted[i]:
				// App started successfully
			case <-time.After(time.Second):
				t.Fatalf("Timeout waiting for app %d to start", i)
			}
		}

		cancel() // Cancel the context to trigger shutdown

		for i := 0; i < appCount; i++ {
			select {
			case <-appFinished[i]:
				// App finished successfully
			case <-time.After(2 * time.Second):
				t.Fatalf("Timeout waiting for app %d to finish", i)
			}
		}

		g.Wait()

		select {
		case err, ok := <-errors:
			if ok {
				if err != context.Canceled && err != nil {
					t.Fatalf("Unexpected error: %v", err)
				}
			}
		case <-time.After(time.Second):
			t.Fatal("Timeout waiting for error channel to close")
		}
	})

	t.Run("Application with error", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		expectedError := fmt.Errorf("test error")

		g := New[string](ctx, map[string]RuntimeApplication{
			"error-app": func(ctx context.Context, shutdown <-chan struct{}) error {
				return expectedError
			},
		})

		errors := g.Start()

		select {
		case err := <-errors:
			if err == nil {
				t.Fatal("Expected an error, got nil")
			}
			if !strings.Contains(err.Error(), "test error") {
				t.Fatalf("Expected error containing 'test error', got %v", err)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("Timeout waiting for error")
		}

		g.Shutdown()
		g.Wait()
	})

	t.Run("Dynamic application addition", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		g := New[string](ctx, nil)
		errors := g.Start()

		appStarted := make(chan struct{})
		appFinished := make(chan struct{})

		err := g.Add("dynamic-app", func(ctx context.Context, shutdown <-chan struct{}) error {
			close(appStarted)
			<-ctx.Done()
			close(appFinished)
			return ctx.Err()
		})
		if err != nil {
			t.Fatalf("Failed to add dynamic app: %v", err)
		}

		select {
		case <-appStarted:
			// App started successfully
		case <-time.After(time.Second):
			t.Fatal("Timeout waiting for dynamic app to start")
		}

		cancel() // Cancel the context to trigger shutdown

		select {
		case <-appFinished:
			// App finished successfully
		case <-time.After(2 * time.Second):
			t.Fatal("Timeout waiting for dynamic app to finish")
		}

		g.Wait()

		select {
		case err, ok := <-errors:
			if ok {
				if err != context.Canceled && err != nil {
					t.Fatalf("Unexpected error: %v", err)
				}
			}
		case <-time.After(time.Second):
			t.Fatal("Timeout waiting for error channel to close")
		}
	})

	t.Run("Race condition test", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		g := New[string](ctx, nil)
		errors := g.Start()

		var wg sync.WaitGroup
		appCount := 100

		for i := 0; i < appCount; i++ {
			wg.Add(1)
			go func(index int) {
				defer wg.Done()
				appKey := fmt.Sprintf("app-%d", index)
				app := func(ctx context.Context, shutdown <-chan struct{}) error {
					<-ctx.Done()
					return ctx.Err()
				}

				if err := g.Add(appKey, app); err != nil {
					t.Errorf("Failed to add app %s: %v", appKey, err)
				}
			}(i)
		}

		wg.Wait()

		if g.keys.Length() != appCount {
			t.Errorf("Expected %d apps, got %d", appCount, g.keys.Length())
		}

		cancel() // Cancel the context to trigger shutdown
		g.Wait()

		select {
		case err, ok := <-errors:
			if ok {
				if err != context.Canceled && err != nil {
					t.Fatalf("Unexpected error: %v", err)
				}
			}
		case <-time.After(time.Second):
			t.Fatal("Timeout waiting for error channel to close")
		}
	})
}

func TestWorker(t *testing.T) {
	t.Run("Basic worker functionality", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		var tickCount int32
		interval := 100 * time.Millisecond
		worker := Worker(interval, NonBlocking, func(ctx context.Context) error {
			atomic.AddInt32(&tickCount, 1)
			return nil
		})

		shutdown := make(chan struct{})
		done := make(chan struct{})

		go func() {
			err := worker(ctx, shutdown)
			if err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
			close(done)
		}()

		time.Sleep(550 * time.Millisecond) // Allow for 5 ticks (with some margin)
		close(shutdown)

		<-done

		finalCount := atomic.LoadInt32(&tickCount)
		if finalCount < 4 || finalCount > 6 {
			t.Errorf("Expected around 5 ticks, got %d", finalCount)
		}
	})

	t.Run("Worker with error", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		expectedError := fmt.Errorf("worker error")
		interval := 100 * time.Millisecond
		worker := Worker(interval, NonBlocking, func(ctx context.Context) error {
			return expectedError
		})

		shutdown := make(chan struct{})
		errChan := make(chan error, 1)

		go func() {
			errChan <- worker(ctx, shutdown)
		}()

		select {
		case err := <-errChan:
			if err != expectedError {
				t.Errorf("Expected error %v, got %v", expectedError, err)
			}
		case <-time.After(1 * time.Second): // Increased timeout
			t.Fatal("Timeout waiting for worker error")
		}
	})

	t.Run("Worker with different execution modes", func(t *testing.T) {
		testCases := []struct {
			name string
			mode ExecutionMode
		}{
			{"NonBlocking", NonBlocking},
			{"ManagedTimeline", ManagedTimeline},
			{"BestEffort", BestEffort},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()

				var tickCount int32
				interval := 100 * time.Millisecond
				worker := Worker(interval, tc.mode, func(ctx context.Context) error {
					atomic.AddInt32(&tickCount, 1)
					time.Sleep(50 * time.Millisecond) // Simulate some work
					return nil
				})

				shutdown := make(chan struct{})
				done := make(chan struct{})

				go func() {
					err := worker(ctx, shutdown)
					if err != nil {
						t.Errorf("Unexpected error: %v", err)
					}
					close(done)
				}()

				time.Sleep(550 * time.Millisecond) // Allow for 5 ticks (with some margin)
				close(shutdown)

				<-done

				finalCount := atomic.LoadInt32(&tickCount)
				if finalCount < 3 || finalCount > 6 {
					t.Errorf("Expected 3-6 ticks for %s mode, got %d", tc.name, finalCount)
				}
			})
		}
	})
}
