package gronos

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClock(t *testing.T) {
	t.Run("AddAndDispatchTicks", func(t *testing.T) {
		clock := NewClock(WithInterval(10 * time.Millisecond))
		ticker := &mockTicker{}
		clock.Add(ticker, NonBlocking)

		clock.Start()
		time.Sleep(100 * time.Millisecond)
		clock.Stop()

		assert.GreaterOrEqual(t, ticker.tickCount.Load(), int64(4), "Expected at least 4 ticks")
	})

	t.Run("MultipleTickerModes", func(t *testing.T) {
		clock := NewClock(WithInterval(10 * time.Millisecond))
		nonBlockingTicker := &mockTicker{}
		managedTicker := &mockTicker{}
		bestEffortTicker := &mockTicker{}

		clock.Add(nonBlockingTicker, NonBlocking)
		clock.Add(managedTicker, ManagedTimeline)
		clock.Add(bestEffortTicker, BestEffort)

		clock.Start()
		time.Sleep(100 * time.Millisecond)
		clock.Stop()

		assert.GreaterOrEqual(t, nonBlockingTicker.tickCount.Load(), int64(4), "Expected at least 4 ticks for NonBlocking")
		assert.GreaterOrEqual(t, managedTicker.tickCount.Load(), int64(4), "Expected at least 4 ticks for ManagedTimeline")
		assert.GreaterOrEqual(t, bestEffortTicker.tickCount.Load(), int64(4), "Expected at least 4 ticks for BestEffort")
	})
}

func TestAppManager(t *testing.T) {
	t.Run("AddAndShutdownApplication", func(t *testing.T) {
		am := NewAppManager[string](5 * time.Second)
		appName := "testApp"

		app := func(ctx context.Context, shutdown chan struct{}) error {
			<-ctx.Done()
			return nil
		}

		err := am.AddApplication(appName, app)
		require.NoError(t, err)

		err = am.ShutdownApplication(appName)
		require.NoError(t, err)
	})

	t.Run("GracefulShutdown", func(t *testing.T) {
		am := NewAppManager[string](1 * time.Second)

		app := func(ctx context.Context, shutdown chan struct{}) error {
			<-ctx.Done()
			time.Sleep(500 * time.Millisecond)
			return nil
		}

		err := am.AddApplication("app1", app)
		require.NoError(t, err)
		err = am.AddApplication("app2", app)
		require.NoError(t, err)

		err = am.GracefulShutdown()
		require.NoError(t, err)
	})
}

func TestMiddlewaresWithAppManager(t *testing.T) {
	tests := []struct {
		name       string
		middleware func(time.Duration, RuntimeApplication) RuntimeApplication
	}{
		{"NonBlockingMiddleware", NonBlockingMiddleware},
		{"ManagedTimelineMiddleware", ManagedTimelineMiddleware},
		{"BestEffortMiddleware", BestEffortMiddleware},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			am := NewAppManager[string](5 * time.Second)
			shutdownChan, errChan := am.Run(nil)
			defer close(shutdownChan)

			executionCount := atomic.Int64{}
			middleware := tt.middleware(50*time.Millisecond, func(ctx context.Context, shutdown chan struct{}) error {
				count := executionCount.Add(1)
				t.Logf("Execution count: %d", count)
				if count == 6 {
					t.Log("Sending test error")
					return fmt.Errorf("test error")
				}
				return nil
			})

			err := am.AddApplication(tt.name, middleware)
			require.NoError(t, err)

			select {
			case err := <-errChan:
				require.Error(t, err)
				require.Contains(t, err.Error(), "test error")
			case <-time.After(2 * time.Second):
				t.Fatal("Timeout waiting for error from AppManager")
			}

			// Wait for the application to be removed
			time.Sleep(100 * time.Millisecond)

			// Check if the application has been removed
			_, loaded := am.apps.Load(tt.name)
			require.False(t, loaded, "Application should have been removed")
		})
	}
}

type mockTicker struct {
	tickCount atomic.Int64
}

func (mt *mockTicker) Tick() {
	mt.tickCount.Add(1)
}

func TestDataRaceFreedom(t *testing.T) {
	t.Run("ClockConcurrency", func(t *testing.T) {
		clock := NewClock(WithInterval(1 * time.Millisecond))
		var wg sync.WaitGroup

		for i := 0; i < 100; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				ticker := &mockTicker{}
				clock.Add(ticker, NonBlocking)
			}()
		}

		clock.Start()
		wg.Wait()
		clock.Stop()
	})

	t.Run("AppManagerConcurrency", func(t *testing.T) {
		am := NewAppManager[string](5 * time.Second)
		var wg sync.WaitGroup

		app := func(ctx context.Context, shutdown chan struct{}) error {
			select {
			case <-ctx.Done():
				return nil
			case <-shutdown:
				return nil
			}
		}

		for i := 0; i < 100; i++ {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				err := am.AddApplication(fmt.Sprintf("app%d", i), app)
				require.NoError(t, err)
			}(i)
		}

		wg.Wait()

		for i := 0; i < 100; i++ {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				err := am.ShutdownApplication(fmt.Sprintf("app%d", i))
				require.NoError(t, err)
			}(i)
		}

		wg.Wait()
	})
}

func TestTickerSubscriberDynamicInterval(t *testing.T) {
	clock := NewClock(WithInterval(5 * time.Millisecond))
	ticker := &mockTicker{}
	clock.Add(ticker, NonBlocking)

	subscriber, _ := clock.subs.Load(ticker)
	ts := subscriber.(*TickerSubscriber)

	ts.DynamicInterval = func(elapsedTime time.Duration) time.Duration {
		return 5 * time.Millisecond
	}

	clock.Start()
	time.Sleep(150 * time.Millisecond)
	clock.Stop()

	assert.GreaterOrEqual(t, ticker.tickCount.Load(), int64(10), "Expected at least 10 ticks with dynamic interval")
}

func TestAppManagerMultipleApplications(t *testing.T) {
	am := NewAppManager[string](5 * time.Second)
	appName1 := "testApp1"
	appName2 := "testApp2"

	app := func(ctx context.Context, shutdown chan struct{}) error {
		<-shutdown
		return nil
	}

	err := am.AddApplication(appName1, app)
	require.NoError(t, err)

	err = am.AddApplication(appName2, app)
	require.NoError(t, err)

	err = am.ShutdownApplication(appName1)
	require.NoError(t, err)

	err = am.ShutdownApplication(appName2)
	require.NoError(t, err)
}

func TestAppManagerGracefulShutdownWithLongRunningApps(t *testing.T) {
	am := NewAppManager[string](2 * time.Second)

	app := func(ctx context.Context, shutdown chan struct{}) error {
		select {
		case <-ctx.Done():
			time.Sleep(1 * time.Second)
			return nil
		case <-shutdown:
			return nil
		}
	}

	err := am.AddApplication("app1", app)
	require.NoError(t, err)
	err = am.AddApplication("app2", app)
	require.NoError(t, err)

	start := time.Now()
	err = am.GracefulShutdown()
	require.NoError(t, err)

	elapsed := time.Since(start)
	assert.Less(t, elapsed, 3*time.Second, "Graceful shutdown should complete within 3 seconds")
}

func TestMiddlewareErrorHandling(t *testing.T) {
	am := NewAppManager[string](5 * time.Second)
	appName := "errorApp"

	app := func(ctx context.Context, shutdown chan struct{}) error {
		return fmt.Errorf("expected error")
	}

	wrappedApp := NonBlockingMiddleware(50*time.Millisecond, app)

	err := am.AddApplication(appName, wrappedApp)
	require.NoError(t, err)

	shutdownChan, errChan := am.Run(nil)

	var receivedErr error
	select {
	case err, ok := <-errChan:
		if !ok {
			t.Fatal("Error channel closed unexpectedly")
		}
		receivedErr = err
	case <-time.After(3 * time.Second):
		t.Fatal("Timeout waiting for error from AppManager")
	}

	assert.Error(t, receivedErr)
	assert.Contains(t, receivedErr.Error(), "expected error")

	close(shutdownChan)
}

func TestMiddlewareTickFrequency(t *testing.T) {
	am := NewAppManager[string](5 * time.Second)
	executionCount := atomic.Int64{}
	appName := "tickFreqApp"

	app := func(ctx context.Context, shutdown chan struct{}) error {
		executionCount.Add(1)
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-shutdown:
			return nil
		case <-time.After(10 * time.Millisecond):
			return nil
		}
	}

	wrappedApp := NonBlockingMiddleware(50*time.Millisecond, app)

	err := am.AddApplication(appName, wrappedApp)
	require.NoError(t, err)

	shutdownChan, _ := am.Run(nil)

	time.Sleep(700 * time.Millisecond)
	close(shutdownChan)

	finalCount := executionCount.Load()
	assert.GreaterOrEqual(t, finalCount, int64(8), "Expected at least 8 ticks within 500ms")
}

func TestApplicationShuttingDownOtherApplications(t *testing.T) {
	am := NewAppManager[string](5 * time.Second)
	shutdownChan, errChan := am.Run(nil)
	defer close(shutdownChan)

	appName := "shutdownTestApp"
	err := am.AddApplication(appName, RuntimeApplication(func(ctx context.Context, shutdown chan struct{}) error {
		select {
		case <-time.After(1 * time.Second):
			t.Logf("Sending shutdown signal for %s", appName)
			return fmt.Errorf("test shutdown")
		case <-shutdown:
			t.Logf("Received shutdown signal for %s", appName)
			return nil
		}
	}))
	require.NoError(t, err)

	select {
	case err := <-errChan:
		require.Error(t, err)
		require.Contains(t, err.Error(), "test shutdown")
	case <-time.After(2 * time.Second):
		t.Fatal("Timeout waiting for error from AppManager")
	}

	// Wait for the application to be removed
	time.Sleep(100 * time.Millisecond)

	// Check if the application has been removed
	_, loaded := am.apps.Load(appName)
	require.False(t, loaded, "Application should have been removed")
}

func TestApplicationAddingOtherApplications(t *testing.T) {
	am := NewAppManager[string](5 * time.Second)

	childAppName := "childApp"
	parentAppName := "parentApp"

	childApp := func(ctx context.Context, shutdown chan struct{}) error {
		t.Log("Child app started")
		select {
		case <-UseWait(ctx, am, childAppName, StateRunning):
			t.Log("Child app running")
		case <-ctx.Done():
			return ctx.Err()
		}

		select {
		case <-shutdown:
			t.Log("Child app received shutdown signal")
		case <-ctx.Done():
			return ctx.Err()
		}

		t.Log("Child app shutting down")
		return nil
	}

	parentApp := func(ctx context.Context, shutdown chan struct{}) error {
		t.Log("Parent app started")
		ctx = context.WithValue(ctx, currentAppKey, parentAppName)
		UseAdd(ctx, am.useAddChan, childAppName, childApp)

		select {
		case <-UseWait(ctx, am, parentAppName, StateRunning):
			t.Log("Parent app running")
		case <-ctx.Done():
			return ctx.Err()
		}

		select {
		case <-UseWait(ctx, am, childAppName, StateRunning):
			t.Log("Child app is now running")
		case <-ctx.Done():
			return ctx.Err()
		}

		t.Log("Parent app initiating shutdown of child")
		UseShutdown(ctx, am.useShutdownChan, childAppName)

		select {
		case <-UseWait(ctx, am, childAppName, StateCompleted):
			t.Log("Child app has completed")
		case <-ctx.Done():
			return ctx.Err()
		}

		select {
		case <-shutdown:
			t.Log("Parent app received shutdown signal")
		case <-ctx.Done():
			return ctx.Err()
		}

		t.Log("Parent app shutting down")
		return nil
	}

	err := am.AddApplication(parentAppName, parentApp)
	require.NoError(t, err)

	shutdownMainChan, errChan := am.Run(nil)

	time.Sleep(1 * time.Second) // Give some time for apps to start and run

	close(shutdownMainChan) // Initiate shutdown of all apps

	select {
	case err := <-errChan:
		if err != nil && !errors.Is(err, context.Canceled) {
			t.Fatalf("Unexpected error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Timeout waiting for applications to shut down")
	}

	// Verify both apps are removed
	_, parentLoaded := am.apps.Load(parentAppName)
	require.False(t, parentLoaded, "Parent app should be unloaded")
	_, childLoaded := am.apps.Load(childAppName)
	require.False(t, childLoaded, "Child app should be unloaded")
}
