package nonos

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type mockTicker struct {
	tickCount atomic.Int32
}

func (m *mockTicker) Tick() {
	m.tickCount.Add(1)
}

func TestClock(t *testing.T) {
	t.Run("Basic functionality", func(t *testing.T) {
		clock := NewClock(WithInterval(10 * time.Millisecond))
		ticker := &mockTicker{}
		clock.Add(ticker, NonBlocking)

		clock.Start()
		time.Sleep(55 * time.Millisecond)
		clock.Stop()

		expectedTicks := int32(5) // Approximately 5 ticks in 55ms with 10ms interval
		actualTicks := ticker.tickCount.Load()
		if actualTicks < expectedTicks-1 || actualTicks > expectedTicks+1 {
			t.Errorf("Expected around %d ticks, got %d", expectedTicks, actualTicks)
		}
	})

	t.Run("Multiple tickers", func(t *testing.T) {
		clock := NewClock(WithInterval(10 * time.Millisecond))
		ticker1 := &mockTicker{}
		ticker2 := &mockTicker{}
		ticker3 := &mockTicker{}

		clock.Add(ticker1, NonBlocking)
		clock.Add(ticker2, ManagedTimeline)
		clock.Add(ticker3, BestEffort)

		clock.Start()
		time.Sleep(65 * time.Millisecond) // Increased from 55ms to 65ms
		clock.Stop()

		expectedTicks := int32(6) // Approximately 6 ticks in 65ms with 10ms interval
		for i, ticker := range []*mockTicker{ticker1, ticker2, ticker3} {
			actualTicks := ticker.tickCount.Load()
			if actualTicks < expectedTicks-2 || actualTicks > expectedTicks+2 {
				t.Errorf("Ticker %d: Expected around %d ticks, got %d", i+1, expectedTicks, actualTicks)
			}
		}
	})

	t.Run("Dynamic interval", func(t *testing.T) {
		clock := NewClock(WithInterval(10 * time.Millisecond))
		ticker := &mockTicker{}

		var lastInterval atomic.Int64
		lastInterval.Store(10)

		dynamicInterval := func(elapsed time.Duration) time.Duration {
			current := lastInterval.Load()
			newInterval := current + 5 // Increase by 5ms each time
			lastInterval.Store(newInterval)
			return time.Duration(current) * time.Millisecond
		}

		sub := &TickerSubscriber{
			Ticker:          ticker,
			Mode:            NonBlocking,
			DynamicInterval: dynamicInterval,
		}
		sub.lastExecTime.Store(time.Now())
		clock.subs.Store(ticker, sub)

		clock.Start()
		time.Sleep(100 * time.Millisecond)
		clock.Stop()

		actualTicks := ticker.tickCount.Load()

		// With the increasing interval, we expect fewer ticks
		if actualTicks < 2 || actualTicks > 4 {
			t.Errorf("Expected 2-4 ticks, got %d", actualTicks)
		}
	})

	t.Run("Concurrent operations", func(t *testing.T) {
		clock := NewClock(WithInterval(1 * time.Millisecond))
		const goroutines = 100
		var wg sync.WaitGroup
		wg.Add(goroutines)

		for i := 0; i < goroutines; i++ {
			go func() {
				defer wg.Done()
				ticker := &mockTicker{}
				clock.Add(ticker, NonBlocking)
			}()
		}

		clock.Start()
		wg.Wait()
		time.Sleep(10 * time.Millisecond)
		clock.Stop()

		var totalTicks int32
		clock.subs.Range(func(_, value interface{}) bool {
			ticker := value.(*TickerSubscriber).Ticker.(*mockTicker)
			totalTicks += ticker.tickCount.Load()
			return true
		})

		if totalTicks == 0 {
			t.Error("Expected some ticks to occur, but none were recorded")
		}
	})
}
