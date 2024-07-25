package nonos

import (
	"sync"
	"sync/atomic"
	"time"
)

type Ticker interface {
	Tick()
}

type ExecutionMode int

const (
	NonBlocking ExecutionMode = iota
	ManagedTimeline
	BestEffort
)

type TickerSubscriber struct {
	Ticker          Ticker
	Mode            ExecutionMode
	lastExecTime    atomic.Value
	DynamicInterval func(lastInterval time.Duration) time.Duration
}

type Clock struct {
	name     string
	interval time.Duration
	ticker   *time.Ticker
	stopCh   chan struct{}
	subs     sync.Map
	ticking  atomic.Bool
	started  atomic.Bool
}

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

type ClockOption func(*Clock)

func WithName(name string) ClockOption {
	return func(c *Clock) {
		c.name = name
	}
}

func WithInterval(interval time.Duration) ClockOption {
	return func(c *Clock) {
		c.interval = interval
	}
}

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

func (c *Clock) Start() {
	if !c.started.CompareAndSwap(false, true) {
		return
	}
	c.ticking.Store(true)
	go c.dispatchTicks()
}

func (c *Clock) Stop() {
	if !c.ticking.CompareAndSwap(true, false) {
		return
	}
	close(c.stopCh)
	c.started.Store(false)
}

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

func (c *Clock) executeTick(key interface{}, sub *TickerSubscriber, now time.Time) {
	sub.Ticker.Tick()
	sub.lastExecTime.Store(now)
	c.subs.Store(key, sub)
}
