package gronos

import (
	"context"
	"fmt"
	"strings"
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

		g, cerrors := New[string](
			ctx,
			map[string]LifecyleFunc{
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
			},
			WithGracePeriod[string](time.Second/2),
		)

		<-appStarted

		g.Shutdown()

		select {
		case <-appFinished:
			// App finished successfully
		case <-time.After(2 * time.Second):
			t.Fatal("Timeout waiting for app to finish")
		}

		g.Wait()

		select {
		case err, ok := <-cerrors:
			if ok {
				if err != nil && err != context.Canceled {
					t.Fatalf("Unexpected error: %v", err)
				}
			}
		case <-time.After(time.Second):
			t.Fatal("Timeout waiting for error channel to close")
		}
	})

	t.Run("Basic removal", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		appStarted := make(chan struct{})
		appFinished := make(chan struct{})

		g, _ := New[string](
			ctx,
			map[string]LifecyleFunc{
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
			},
			WithShutdownBehavior[string](ShutdownAutomatic),
		)

		<-appStarted

		_, msg := msgTerminatedShutdown("test-app")
		g.sendMessage(g.getSystemMetadata(), msg)

		removed, msgr := MsgRemove("test-app")
		g.sendMessage(g.getSystemMetadata(), msgr)
		<-removed

		data, msgg := MsgRequestGraph[string]()
		g.Send(msgg)
		state := <-data

		if state.Size() != 0 {
			t.Fatalf("Expected 0 applications, got %d", state.Size())
		}

		select {
		case <-appFinished:
			// App finished successfully
		case <-time.After(5 * time.Second):
			t.Fatal("Timeout waiting for app to finish")
		}

		g.Wait()

	})

	t.Run("Multiple applications", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		appCount := 3
		appStarted := make([]chan struct{}, appCount)
		appFinished := make([]chan struct{}, appCount)

		apps := make(map[string]LifecyleFunc)
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

		g, cerrors := New[string](ctx, apps)

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
		case err, ok := <-cerrors:
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

		g, cerrors := New[string](ctx, map[string]LifecyleFunc{
			"error-app": func(ctx context.Context, shutdown <-chan struct{}) error {
				return expectedError
			},
		},
			WithoutMinRuntime[string]())

		select {
		case err := <-cerrors:
			fmt.Println(err)
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

		g, cerrors := New[string](ctx, nil)

		appStarted := make(chan struct{})
		appFinished := make(chan struct{})

		g.Add("dynamic-app", func(ctx context.Context, shutdown <-chan struct{}) error {
			close(appStarted)
			<-ctx.Done()
			close(appFinished)
			return ctx.Err()
		})

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
		case err, ok := <-cerrors:
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

		tickCount := atomic.Int32{}
		tickChan := make(chan struct{})

		worker := Worker(time.Millisecond, NonBlocking, func(ctx context.Context) error {
			tickCount.Add(1)
			select {
			case tickChan <- struct{}{}:
			default:
			}
			return nil
		})

		go worker(ctx, nil)

		// Wait for 5 ticks
		for i := 0; i < 5; i++ {
			select {
			case <-tickChan:
			case <-time.After(time.Second):
				t.Fatalf("Timed out waiting for tick %d", i+1)
			}
		}

		cancel()

		time.Sleep(10 * time.Millisecond) // Short sleep to allow for any final ticks

		finalCount := tickCount.Load()
		t.Logf("Final tick count: %d", finalCount)

		if finalCount < 5 {
			t.Errorf("Expected at least 5 ticks, got %d", finalCount)
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

	t.Run("Workers shutdown different times with shutdown", func(t *testing.T) {
		var testCases []struct {
			instances          int
			timeShutdowns      []time.Duration
			timeAfterShutdowns []time.Duration
			behaviour          ShutdownBehavior
			gracePeriod        time.Duration
			shutdown           bool
		}

		behaviours := []ShutdownBehavior{ShutdownAutomatic, ShutdownManual}

		// let's test all combinations
		for _, behaviour := range behaviours {
			testCases = append(testCases, struct {
				instances          int
				timeShutdowns      []time.Duration
				timeAfterShutdowns []time.Duration
				behaviour          ShutdownBehavior
				gracePeriod        time.Duration
				shutdown           bool
			}{
				instances:          3,
				timeShutdowns:      []time.Duration{1, 2, 2},
				timeAfterShutdowns: []time.Duration{1, 2, 2},
				gracePeriod:        3 * time.Second,
				behaviour:          behaviour,
				shutdown:           true,
			})
		}

		testCases = append(testCases, struct {
			instances          int
			timeShutdowns      []time.Duration
			timeAfterShutdowns []time.Duration
			behaviour          ShutdownBehavior
			gracePeriod        time.Duration
			shutdown           bool
		}{
			instances:          3,
			timeShutdowns:      []time.Duration{1, 2, 2},
			timeAfterShutdowns: []time.Duration{1, 2, 2},
			gracePeriod:        3 * time.Second,
			behaviour:          ShutdownAutomatic,
			shutdown:           false,
		})

		// We have 3 tests
		// Automatic means it will continously try to see if there are still running applications
		// - Automatic + Waiting + Shutdown: 			perfect
		// - Automatic + Waiting + No Shutdown: 		should works
		// Manual means it doesn't know when to stop, you have to call Shutdown
		// - Manual + Waiting + Shutdown:				perfect
		// Any other cases are failures or the user that didn't read nor understood the documentation OR do it on purpose
		for _, tc := range testCases {
			t.Run(fmt.Sprintf("Instances %d - behaviour %v with shutdown %v", tc.instances, tc.behaviour, tc.shutdown), func(t *testing.T) {
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()

				endedAfter := make([]time.Duration, tc.instances)
				endedPostShutdown := make([]time.Duration, tc.instances)

				runtimeApplications := make(map[string]LifecyleFunc)
				for i := 0; i < tc.instances; i++ {
					index := i
					runtimeApplications[fmt.Sprintf("app-%d", i)] = func(ctx context.Context, shutdown <-chan struct{}) error {
						now := time.Now()
						defer func() {
							endedPostShutdown[index] = time.Since(now)
						}()
						select {
						case <-time.After(tc.timeShutdowns[index] * time.Second):
							endedAfter[index] = time.Since(now)
							now = time.Now()
							<-time.After(tc.timeAfterShutdowns[index] * time.Second)
							return nil
						case <-shutdown:
							return nil
						}
					}
				}

				g, cerrors := New[string](
					ctx,
					runtimeApplications,
					WithMinRuntime[string](time.Second/2),
					WithGracePeriod[string](tc.gracePeriod),
					WithImmediatePeriod[string](time.Second/2),
					WithShutdownBehavior[string](tc.behaviour),
				)

				go func() {
					for err := range cerrors {
						if err != nil {
							t.Errorf("Unexpected error: %v", err)
						}
					}
				}()

				<-time.After(2 * time.Second)

				if tc.shutdown {
					fmt.Println("g.Shutdown()")
					g.Shutdown()
				}

				fmt.Println("g.Wait()")
				g.Wait()

				tolerance := 500 * time.Millisecond

				for i, duration := range endedAfter {
					if duration < tc.timeShutdowns[i]*time.Second-tolerance {
						t.Errorf("App %d ended too soon: %v", i, duration)
					}
					fmt.Println("App", i, "ended after", duration)
				}

				for i, duration := range endedPostShutdown {
					if duration < tc.timeAfterShutdowns[i]*time.Second-tolerance {
						t.Errorf("App %d ended too soon after shutdown: %v", i, duration)
					}
					fmt.Println("App", i, "ended after shutdown", duration)
				}
			})
		}

	})
}
