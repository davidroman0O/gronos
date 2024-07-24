package gronos

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockTicker struct {
	tickCount atomic.Int64
}

func (mt *mockTicker) Tick() {
	mt.tickCount.Add(1)
}

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

		app := func(ctx context.Context, shutdown chan struct{}) (<-chan error, error) {
			errChan := make(chan error)
			go func() {
				<-ctx.Done()
				close(errChan)
			}()
			return errChan, nil
		}

		err := am.AddApplication(appName, app)
		require.NoError(t, err)

		err = am.ShutdownApplication(appName)
		require.NoError(t, err)
	})

	t.Run("GracefulShutdown", func(t *testing.T) {
		am := NewAppManager[string](1 * time.Second)

		app := func(ctx context.Context, shutdown chan struct{}) (<-chan error, error) {
			errChan := make(chan error)
			go func() {
				<-ctx.Done()
				time.Sleep(500 * time.Millisecond)
				close(errChan)
			}()
			return errChan, nil
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
		name             string
		middleware       func(time.Duration, RuntimeApplication) RuntimeApplication
		expectedMinExecs int64
	}{
		{"NonBlockingMiddleware", NonBlockingMiddleware, 5},
		{"ManagedTimelineMiddleware", ManagedTimelineMiddleware, 3},
		{"BestEffortMiddleware", BestEffortMiddleware, 5},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			am := NewAppManager[string](5 * time.Second)
			executionCount := atomic.Int64{}
			errorReceived := make(chan struct{}, 1)

			app := func(ctx context.Context, shutdown chan struct{}) (<-chan error, error) {
				errChan := make(chan error, 1)
				go func() {
					defer close(errChan)
					for i := 0; i < 10; i++ {
						select {
						case <-ctx.Done():
							return
						case <-shutdown:
							return
						case <-time.After(10 * time.Millisecond):
							executionCount.Add(1)
							if i == 5 {
								errChan <- fmt.Errorf("test error")
								close(errorReceived)
								return
							}
						}
					}
				}()
				return errChan, nil
			}

			wrappedApp := tc.middleware(50*time.Millisecond, app)

			err := am.AddApplication(tc.name, wrappedApp)
			require.NoError(t, err)

			shutdownChan, errChan := am.Run(nil)

			select {
			case <-errorReceived:
				// Error was sent, now wait for it to propagate through AppManager
			case <-time.After(1 * time.Second):
				t.Fatal("Timeout waiting for error to be triggered")
			}

			select {
			case err := <-errChan:
				assert.Error(t, err)
				assert.Contains(t, err.Error(), "test error")
			case <-time.After(1 * time.Second):
				t.Fatal("Timeout waiting for error from AppManager")
			}

			close(shutdownChan)
			// Drain the remaining errors to avoid closing a closed channel
			for range errChan {
			}

			assert.GreaterOrEqual(t, executionCount.Load(), tc.expectedMinExecs,
				"Expected at least %d executions before error", tc.expectedMinExecs)
		})
	}
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

		app := func(ctx context.Context, shutdown chan struct{}) (<-chan error, error) {
			return make(chan error), nil
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
