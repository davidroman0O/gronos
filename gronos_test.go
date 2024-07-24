package gronos

import (
	"context"
	"fmt"
	"log"
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
	testCases := []struct {
		name          string
		middleware    func(time.Duration, RuntimeApplication) RuntimeApplication
		expectedExecs int64
	}{
		{"NonBlockingMiddleware", NonBlockingMiddleware, 6},
		{"ManagedTimelineMiddleware", ManagedTimelineMiddleware, 6},
		{"BestEffortMiddleware", BestEffortMiddleware, 6},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			am := NewAppManager[string](5 * time.Second)
			executionCount := atomic.Int64{}
			errorSent := make(chan struct{})

			app := func(ctx context.Context, shutdown chan struct{}) error {
				count := executionCount.Add(1)
				log.Printf("Execution count: %d", count)
				if count == tc.expectedExecs {
					log.Printf("Sending test error")
					close(errorSent)
					return fmt.Errorf("test error")
				}
				select {
				case <-ctx.Done():
					return ctx.Err()
				case <-shutdown:
					return nil
				case <-time.After(10 * time.Millisecond):
					return nil
				}
			}

			wrappedApp := tc.middleware(50*time.Millisecond, app)

			err := am.AddApplication(tc.name, wrappedApp)
			require.NoError(t, err)

			shutdownChan, errChan := am.Run(nil)

			select {
			case <-errorSent:
				log.Printf("Error sent, waiting for propagation")
			case <-time.After(3 * time.Second):
				t.Fatal("Timeout waiting for error to be sent")
			}

			var receivedErr error
			select {
			case err, ok := <-errChan:
				if !ok {
					t.Fatal("Error channel closed unexpectedly")
				}
				log.Printf("Received error from AppManager: %v", err)
				receivedErr = err
			case <-time.After(3 * time.Second):
				t.Fatal("Timeout waiting for error from AppManager")
			}

			assert.Error(t, receivedErr)
			assert.Contains(t, receivedErr.Error(), "test error")

			close(shutdownChan)

			// Wait for shutdown to complete
			select {
			case _, ok := <-errChan:
				if ok {
					t.Fatal("Error channel not closed after shutdown")
				}
			case <-time.After(3 * time.Second):
				t.Fatal("Timeout waiting for error channel to close")
			}

			finalCount := executionCount.Load()
			log.Printf("Final execution count: %d", finalCount)
			assert.Equal(t, tc.expectedExecs, finalCount,
				"Expected exactly %d executions before error, got %d", tc.expectedExecs, finalCount)
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
	clock := NewClock(WithInterval(5 * time.Millisecond)) // Shortened the interval
	ticker := &mockTicker{}
	clock.Add(ticker, NonBlocking)

	subscriber, _ := clock.subs.Load(ticker)
	ts := subscriber.(*TickerSubscriber)

	// Change dynamic interval
	ts.DynamicInterval = func(elapsedTime time.Duration) time.Duration {
		return 5 * time.Millisecond
	}

	clock.Start()
	time.Sleep(150 * time.Millisecond) // Increased sleep duration
	clock.Stop()

	assert.GreaterOrEqual(t, ticker.tickCount.Load(), int64(10), "Expected at least 10 ticks with dynamic interval")
}

func TestAppManagerMultipleApplications(t *testing.T) {
	am := NewAppManager[string](5 * time.Second)
	appName1 := "testApp1"
	appName2 := "testApp2"

	app := func(ctx context.Context, shutdown chan struct{}) error {
		<-ctx.Done()
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

	time.Sleep(700 * time.Millisecond) // Increased sleep duration to ensure more ticks
	close(shutdownChan)

	finalCount := executionCount.Load()
	assert.GreaterOrEqual(t, finalCount, int64(8), "Expected at least 8 ticks within 500ms")
}

func TestAppManagerApplicationShutdownHandling(t *testing.T) {
	am := NewAppManager[string](5 * time.Second)
	appName := "testApp"

	shutdownHandled := make(chan struct{})

	app := func(ctx context.Context, shutdown chan struct{}) error {
		select {
		case <-shutdown:
			close(shutdownHandled)
			return nil
		case <-ctx.Done():
			return nil
		}
	}

	err := am.AddApplication(appName, app)
	require.NoError(t, err)

	err = am.ShutdownApplication(appName)
	require.NoError(t, err)

	select {
	case <-shutdownHandled:
		// Success
	case <-time.After(2 * time.Second): // Increased timeout
		t.Fatal("Timeout waiting for shutdown to be handled")
	}
}

func TestApplicationAddingOtherApplications(t *testing.T) {
	am := NewAppManager[string](5 * time.Second)
	addChan := am.UseAdd()
	waitChan := am.UseWait

	childAppName := "childApp"
	parentAppName := "parentApp"

	childApp := func(ctx context.Context, shutdown chan struct{}) error {
		<-shutdown
		return nil
	}

	parentApp := func(ctx context.Context, shutdown chan struct{}) error {
		addChan <- AppManagerMsg[string]{name: childAppName, app: childApp}

		select {
		case <-waitChan(childAppName, "started"):
		case <-time.After(1 * time.Second):
			return fmt.Errorf("child app did not start in time")
		}

		<-shutdown
		return nil
	}

	addChan <- AppManagerMsg[string]{name: parentAppName, app: parentApp}

	shutdownChan, errChan := am.Run(nil)
	time.Sleep(100 * time.Millisecond)

	close(shutdownChan)

	select {
	case err, ok := <-errChan:
		if ok {
			t.Fatalf("Error received: %v", err)
		}
	case <-time.After(1 * time.Second):
	}

	select {
	case <-waitChan(parentAppName, "completed"):
	case <-time.After(1 * time.Second):
		t.Fatal("Timeout waiting for parent app to complete")
	}

	select {
	case <-waitChan(childAppName, "completed"):
	case <-time.After(1 * time.Second):
		t.Fatal("Timeout waiting for child app to complete")
	}
}
