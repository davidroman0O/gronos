package gronos

import (
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
	lastExecTime    time.Time
	Priority        int
	DynamicInterval func(elapsedTime time.Duration) time.Duration
}

type TickerSubscriberOption func(*TickerSubscriber)

func WithPriority(priority int) TickerSubscriberOption {
	return func(ts *TickerSubscriber) {
		ts.Priority = priority
	}
}

func WithDynamicInterval(dynamicInterval func(elapsedTime time.Duration) time.Duration) TickerSubscriberOption {
	return func(ts *TickerSubscriber) {
		ts.DynamicInterval = dynamicInterval
	}
}

// Clock is a simple ticker to give you control on when to trigger the sub(s).
// Observation shown that one goroutine to trigger multiple other tickers is more efficient than multiple goroutines.
type Clock struct {
	ticker   *time.Ticker
	stopCh   chan struct{}
	subs     []TickerSubscriber
	interval time.Duration
}

func NewClock(interval time.Duration) *Clock {
	tm := &Clock{
		ticker:   time.NewTicker(interval),
		stopCh:   make(chan struct{}),
		interval: interval,
	}
	return tm
}

func (tm *Clock) Start() {
	go tm.dispatchTicks()
}

func (tm *Clock) Add(rb Ticker, mode ExecutionMode, opts ...TickerSubscriberOption) {
	sub := TickerSubscriber{
		Ticker: rb,
		Mode:   mode,
	}

	for _, opt := range opts {
		opt(&sub)
	}

	tm.subs = append(tm.subs, sub)
}

func (tm *Clock) dispatchTicks() {
	for {
		select {
		case <-tm.ticker.C:
			now := time.Now()
			for i := range tm.subs {
				sub := &tm.subs[i]

				interval := tm.interval
				if sub.DynamicInterval != nil {
					elapsedTime := now.Sub(sub.lastExecTime)
					interval = sub.DynamicInterval(elapsedTime)
				}

				switch sub.Mode {
				case NonBlocking:
					go sub.Ticker.Tick()
				case ManagedTimeline:
					if now.Sub(sub.lastExecTime) >= interval {
						sub.Ticker.Tick()
						sub.lastExecTime = now
					}
				case BestEffort:
					if now.Sub(sub.lastExecTime) >= interval {
						sub.Ticker.Tick()
						sub.lastExecTime = now
					} else {
						continue // Skip execution if elapsed time is less than the interval
					}
				}
			}

		case <-tm.stopCh:
			tm.ticker.Stop()
			return
		}
	}
}

func (tm *Clock) Stop() {
	close(tm.stopCh)
}