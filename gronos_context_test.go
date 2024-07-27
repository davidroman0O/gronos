package gronos

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

func TestGronosContextCancellation(t *testing.T) {

	t.Run("Context cancellation stops all applications", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		appCount := 3
		runningApps := atomic.Int32{}
		completedApps := atomic.Int32{}

		apps := make(map[string]RuntimeApplication)
		for i := 0; i < appCount; i++ {
			apps[string(rune('A'+i))] = func(ctx context.Context, shutdown <-chan struct{}) error {
				runningApps.Add(1)
				defer runningApps.Add(-1)
				defer completedApps.Add(1)

				select {
				case <-ctx.Done():
					return ctx.Err()
				case <-shutdown:
					return nil
				}
			}
		}

		g := New[string](ctx, apps)
		errChan := g.Start()

		// Wait for all apps to start
		for i := 0; i < 100 && runningApps.Load() < int32(appCount); i++ {
			time.Sleep(10 * time.Millisecond)
		}

		cancel() // Cancel the context

		// Wait for all apps to complete with a timeout
		timeout := time.After(2 * time.Second)
		for completedApps.Load() < int32(appCount) {
			select {
			case <-timeout:
				t.Fatalf("Timeout waiting for apps to complete. Completed: %d/%d", completedApps.Load(), appCount)
			default:
				time.Sleep(10 * time.Millisecond)
			}
		}

		g.Wait()

		// Check for errors
		select {
		case err := <-errChan:
			if err != context.Canceled {
				t.Errorf("Expected context.Canceled error, but got: %v", err)
			}
		case <-time.After(time.Second):
			t.Error("Timeout waiting for error after context cancellation")
		}
	})

	t.Run("Context cancellation with long-running application", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		appStarted := make(chan struct{})
		appFinished := make(chan struct{})

		app := func(ctx context.Context, shutdown <-chan struct{}) error {
			close(appStarted)
			select {
			case <-ctx.Done():
				close(appFinished)
				return ctx.Err()
			case <-shutdown:
				close(appFinished)
				return nil
			}
		}

		g := New(ctx, map[string]RuntimeApplication{"long-running": app})
		errChan := g.Start()

		<-appStarted
		cancel()

		select {
		case <-appFinished:
			// App finished as expected
		case <-time.After(2 * time.Second):
			t.Fatal("Timeout waiting for app to finish after context cancellation")
		}

		g.Wait()

		select {
		case err := <-errChan:
			if err != context.Canceled {
				t.Errorf("Expected context.Canceled error, but got: %v", err)
			}
		case <-time.After(time.Second):
			t.Error("Timeout waiting for error after context cancellation")
		}
	})

	t.Run("Adding application after context cancellation", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())

		g := New[string](ctx, nil)
		errChan := g.Start()

		// Cancel the context immediately
		cancel()

		// Try to add an application after cancellation
		app := func(ctx context.Context, shutdown <-chan struct{}) error {
			t.Error("This application should not run")
			return nil
		}

		err := g.Add("late-app", app)
		if err == nil {
			t.Errorf("Expected error when adding app after cancellation")
		}

		g.Wait()

		// Check for errors
		select {
		case err := <-errChan:
			if err == nil || err.Error() != "context canceled" {
				t.Errorf("Expected 'context canceled' error, but got: %v", err)
			}
		default:
			t.Error("Expected an error due to context cancellation, but no error was received")
		}
	})

	t.Run("Worker respects context cancellation", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		tickCount := atomic.Int32{}
		workerApp := Worker(100*time.Millisecond, NonBlocking, func(ctx context.Context) error {
			tickCount.Add(1)
			return nil
		})

		g := New(ctx, map[string]RuntimeApplication{"worker": workerApp})
		errChan := g.Start()

		// Allow some ticks to occur
		time.Sleep(250 * time.Millisecond)

		// Cancel the context
		cancel()

		g.Wait()

		finalCount := tickCount.Load()
		if finalCount < 2 || finalCount > 3 {
			t.Errorf("Expected 2-3 ticks before cancellation, got %d", finalCount)
		}

		// Check for errors
		select {
		case err := <-errChan:
			if err == nil || err.Error() != "context canceled" {
				t.Errorf("Expected 'context canceled' error, but got: %v", err)
			}
		default:
			t.Error("Expected an error due to context cancellation, but no error was received")
		}
	})
}
