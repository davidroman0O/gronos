package clock

import (
	"fmt"
	"time"
)

// By default, the ticking occurs every 100ms so give time to subscribers to accumulate messages and process them.
var DefaultClock = New(WithInterval(100 * time.Millisecond))

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
	name     string
	interval time.Duration
	ticker   *time.Ticker
	stopCh   chan struct{}
	subs     []TickerSubscriber
	ticking  bool
	started  bool
}

func (tm *Clock) Ticking() bool {
	return tm.ticking
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

func New(opts ...ClockOption) *Clock {
	tm := &Clock{
		// ticker:   time.NewTicker(interval),
		stopCh: make(chan struct{}),
		// interval: interval,
		subs:    []TickerSubscriber{},
		ticking: false,
		started: false,
	}
	for _, opt := range opts {
		opt(tm)
	}
	if tm.interval == 0 {
		tm.interval = 100 * time.Millisecond
	}
	tm.ticker = time.NewTicker(tm.interval)
	return tm
}

func (tm *Clock) Start() {
	fmt.Println(tm.name, "=> a clock called to start")
	if tm.started {
		fmt.Println(tm.name, "==> Clock already started")
		// it was a pause
		if !tm.ticking {
			fmt.Println(tm.name, "===> Clock was paused")
			go tm.dispatchTicks()
		}
		return
	}
	if tm.stopCh == nil {
		tm.stopCh = make(chan struct{})
	}
	fmt.Println(tm.name, "> Clock started")
	tm.started = true
	go tm.dispatchTicks()
}

func (tm *Clock) Clear() {
	tm.subs = []TickerSubscriber{}
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
	defer func() {
		fmt.Println(tm.name, "Clock stopped")
	}()
	tm.ticking = true
	fmt.Println(tm.name, "====> Clock ticking")
	for tm.ticking {
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
			tm.ticking = false
			return
		}
	}
}

func (tm *Clock) Pause() {
	tm.ticking = false
	tm.ticker.Stop()
}

func (tm *Clock) Stop() {
	close(tm.stopCh)
}
