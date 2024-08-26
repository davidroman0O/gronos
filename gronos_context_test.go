package gronos

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"
	"time"
)

func TestGronosContextCancellation(t *testing.T) {

	t.Run("Context cancellation stops all applications", func(t *testing.T) {
		// Create a context with a timeout to ensure the test doesn't hang
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		appCount := 3
		apps := make(map[string]LifecyleFunc)
		appStatuses := make(map[string]*atomic.Int32)

		for i := 0; i < appCount; i++ {
			appName := fmt.Sprintf("App%d", i)
			status := &atomic.Int32{}
			appStatuses[appName] = status
			apps[appName] = func(appCtx context.Context, shutdown <-chan struct{}) error {
				status.Store(1)       // App is running
				defer status.Store(2) // App has stopped
				select {
				case <-appCtx.Done():
					return appCtx.Err()
				case <-shutdown:
					return nil
				}
			}
		}

		g, errChan := New[string](ctx, apps)

		for {
			allRunning := true
			for _, status := range appStatuses {
				if status.Load() != 1 {
					allRunning = false
					break
				}
			}
			if allRunning {
				break
			}
			time.Sleep(100 * time.Millisecond)
		}

		// Cancel the context
		t.Log("Cancelling context")
		cancel()

		// Check for the expected error
		select {
		case err := <-errChan:
			fmt.Println("Received error from Gronos:", err)
			if err != context.Canceled && err != context.DeadlineExceeded {
				t.Errorf("Expected context.Canceled or context.DeadlineExceeded, got: %v", err)
			}
		case <-time.After(3 * time.Second):
			t.Error("Timed out waiting for error from Gronos")
		}

		// Ensure Gronos has fully shut down
		g.Wait()

		t.Log("Test completed successfully")
	})

	t.Run("Context cancellation with long-running application", func(t *testing.T) {

		appStarted := make(chan struct{})
		appFinished := make(chan struct{})

		app := func(ctx context.Context, shutdown <-chan struct{}) error {
			close(appStarted)
			defer fmt.Println("App received context cancellation")
			select {
			case <-ctx.Done():
				close(appFinished)
				return ctx.Err()
			case <-shutdown:
				close(appFinished)
				return nil
			}
		}

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		g, errChan := New(ctx, map[string]LifecyleFunc{"long-running": app})

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
			if err == nil {
				t.Error("Expected an error, but got nil")
			}
			if err != context.Canceled {
				t.Errorf("Expected context.Canceled error, but got: %v", err)
			}
		case <-time.After(time.Second):
			t.Error("Timeout waiting for error after context cancellation")
		}
	})

	t.Run("Adding application after context cancellation", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())

		g, errChan := New[string](ctx, nil)

		// Cancel the context immediately
		cancel()

		// Try to add an application after cancellation
		app := func(ctx context.Context, shutdown <-chan struct{}) error {
			t.Error("This application should not run")
			return nil
		}

		g.Add("late-app", app)

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

		g, errChan := New(ctx, map[string]LifecyleFunc{"worker": workerApp})

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
