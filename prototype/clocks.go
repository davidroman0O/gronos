// Package gronos provides a concurrent application management system.
package gronos

import (
	"sync"
	"sync/atomic"
	"time"
)

// Ticker interface represents an object that can be ticked.
type Ticker interface {
	// Tick is called when the ticker is triggered.
	Tick()
}

// ExecutionMode defines how a ticker should be executed.
type ExecutionMode int

const (
	// NonBlocking mode executes the ticker without waiting for completion.
	NonBlocking ExecutionMode = iota
	// ManagedTimeline mode ensures tickers are executed in order, potentially delaying subsequent ticks.
	ManagedTimeline
	// BestEffort mode attempts to execute tickers on time but may skip ticks if the system is overloaded.
	BestEffort
)

// TickerSubscriber represents a subscriber to the clock's ticks.
type TickerSubscriber struct {
	Ticker          Ticker
	Mode            ExecutionMode
	lastExecTime    atomic.Value
	DynamicInterval func(lastInterval time.Duration) time.Duration
}

// Clock represents a clock that can manage multiple tickers with different execution modes.
type Clock struct {
	name     string
	interval time.Duration
	ticker   *time.Ticker
	stopCh   chan struct{}
	subs     sync.Map
	ticking  atomic.Bool
	started  atomic.Bool
}

// NewClock creates a new Clock instance with the given options.
//
// Example usage:
//
//	clock := NewClock(
//		WithName("MyClock"),
//		WithInterval(time.Second),
//	)
func NewClock(opts ...ClockOption) *Clock {
	c := &Clock{
		interval: 100 * time.Millisecond,
		stopCh:   make(chan struct{}),
	}
	for _, opt := range opts {
		opt(c)
	}
	c.ticker = time.NewTicker(c.interval)
	return c
}

// ClockOption is a function type for configuring a Clock instance.
type ClockOption func(*Clock)

// WithName sets the name of the Clock.
func WithName(name string) ClockOption {
	return func(c *Clock) {
		c.name = name
	}
}

// WithInterval sets the tick interval of the Clock.
func WithInterval(interval time.Duration) ClockOption {
	return func(c *Clock) {
		c.interval = interval
	}
}

// Add subscribes a Ticker to the Clock with the specified ExecutionMode.
//
// Example usage:
//
//	clock.Add(&MyTicker{}, NonBlocking)
func (c *Clock) Add(ticker Ticker, mode ExecutionMode) {
	sub := &TickerSubscriber{
		Ticker: ticker,
		Mode:   mode,
		DynamicInterval: func(lastInterval time.Duration) time.Duration {
			return c.interval
		},
	}
	sub.lastExecTime.Store(time.Now())
	c.subs.Store(ticker, sub)
}

// Start begins the Clock's ticking process.
//
// Example usage:
//
//	clock.Start()
func (c *Clock) Start() {
	if !c.started.CompareAndSwap(false, true) {
		return
	}
	c.ticking.Store(true)
	go c.dispatchTicks()
}

// Stop halts the Clock's ticking process.
//
// Example usage:
//
//	clock.Stop()
func (c *Clock) Stop() {
	if !c.ticking.CompareAndSwap(true, false) {
		return
	}
	close(c.stopCh)
	c.started.Store(false)
}

// dispatchTicks is the main loop that handles ticking and subscriber execution.
func (c *Clock) dispatchTicks() {
	nextTick := time.Now().Add(c.interval)
	for c.ticking.Load() {
		now := time.Now()
		if now.Before(nextTick) {
			time.Sleep(nextTick.Sub(now))
			continue
		}

		c.subs.Range(func(key, value interface{}) bool {
			sub := value.(*TickerSubscriber)
			lastExecTime := sub.lastExecTime.Load().(time.Time)
			elapsedTime := now.Sub(lastExecTime)
			interval := sub.DynamicInterval(c.interval)
			if elapsedTime >= interval-time.Millisecond {
				go c.executeTick(key, sub, now)
			}
			return true
		})

		nextTick = nextTick.Add(c.interval)
	}
}

// executeTick performs the actual execution of a subscriber's Tick method.
func (c *Clock) executeTick(key interface{}, sub *TickerSubscriber, now time.Time) {
	sub.Ticker.Tick()
	sub.lastExecTime.Store(now)
	c.subs.Store(key, sub)
}
