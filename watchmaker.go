package gronos

import (
	"fmt"
	"reflect"
	"time"

	"github.com/davidroman0O/gronos/clock"
)

type WatchMaker struct {
	registry map[uint]watcher
	*clock.Clock
	*flipID
	time.Duration
	clockOptions []clock.ClockOption
}

type WatchMakerOption func(*WatchMaker)

func WatchMakerWithDuration(duration time.Duration) WatchMakerOption {
	return func(w *WatchMaker) {
		w.clockOptions = append(w.clockOptions, clock.WithInterval(duration))
	}
}

func WatchMakerWithClockName(name string) WatchMakerOption {
	return func(w *WatchMaker) {
		w.clockOptions = append(w.clockOptions, clock.WithName(name))
	}
}

type watchState int

const (
	WatchStateStopped watchState = iota
	WatchStateRunning
	WatchStatePaused
)

type watcher struct {
	id uint
	*clock.Clock
	state watchState
	clock.Ticker
}

// TODO: add options
func NewWatchMaker(opts ...WatchMakerOption) *WatchMaker {
	w := &WatchMaker{
		registry: make(map[uint]watcher),
		flipID:   newFlipID(),
	}
	for _, opt := range opts {
		opt(w)
	}
	if len(w.clockOptions) > 0 {
		w.Clock = clock.New(w.clockOptions...)
	} else if w.Clock == nil {
		w.Clock = clock.DefaultClock // default clock
	}
	// yes even the WatchMaker got it's own clock
	w.Clock.Add(w, clock.ManagedTimeline)
	w.Clock.Start()
	return w
}

// Simply register a ticker with a clock
// TODO: be able to re-use clocks
func (w *WatchMaker) Register(t clock.Ticker, opts ...clock.ClockOption) uint {
	id := w.flipID.Next()
	if len(opts) == 0 {
		opts = append(opts, clock.WithInterval(time.Microsecond*100))
	}
	for _, fn := range opts {
		if reflect.TypeOf(fn) == reflect.TypeOf(clock.WithInterval) {

		}
	}
	ck := clock.New(opts...)
	ck.Add(t, clock.ManagedTimeline)
	wat := watcher{
		id:     id,
		Clock:  ck,
		state:  WatchStateStopped,
		Ticker: t,
	}
	w.registry[id] = wat
	return id
}

func (w *WatchMaker) Tick() {
	for id, info := range w.registry {
		switch info.state {
		case WatchStateStopped:
			defer func() {
				info.state = WatchStateRunning
				w.registry[id] = info
			}()
			info.Clock.Add(info.Ticker, clock.ManagedTimeline)
			// We might already share the clock, so we need to add it
			if !info.Clock.Ticking() {
				info.Clock.Start() // when then it will start FOR REAL
			}
			fmt.Println("WatchMaker started", id)
		}
	}
}

// Start the clock of the watchmaker
func (w *WatchMaker) Start() {
	w.Clock.Start()
}

// Pause the clock of a specific clock
func (w *WatchMaker) Pause(id uint) {
	w.registry[id].Clock.Pause()
}

// Resume the clock of a specific clock
func (w *WatchMaker) Resume(id uint) {
	w.registry[id].Clock.Start()
}
