package gronos

import (
	"fmt"
	"testing"
	"time"

	"github.com/davidroman0O/gronos/clock"
)

type testTicker struct {
	triggered *bool
	count     *int
}

func (t testTicker) Tick() {
	if t.triggered != nil {
		(*t.triggered) = true
	}
	if t.count != nil {
		(*t.count)++
	}
	fmt.Println("tick")
}

func TestWatchMakerSimple(t *testing.T) {
	w := NewWatchMaker()
	if w == nil {
		t.Fatal("WatchMaker is nil")
	}
	indicator := false
	tickerrrrrr := testTicker{
		triggered: &indicator,
	}
	w.Register(tickerrrrrr)
	w.Start()
	time.Sleep(time.Second * 1)
	if !*tickerrrrrr.triggered {
		t.Fatal("tickerrrrrr not triggered")
	}
}

func TestWatchMakerLifecycle(t *testing.T) {
	w := NewWatchMaker(
		WatchMakerWithDuration(time.Millisecond*10),
		WatchMakerWithClockName("watchmaker"),
	)
	if w == nil {
		t.Fatal("WatchMaker is nil")
	}
	counter := 0
	indicator := false
	cycle := testTicker{
		triggered: &indicator,
		count:     &counter,
	}
	id := w.Register(cycle, clock.WithName("cycle"), clock.WithInterval(time.Millisecond*10))
	// count == 0
	time.Sleep(time.Second * 1)
	w.Pause(id)
	// count == 1
	if !*cycle.triggered {
		t.Fatal("tickerrrrrr not triggered")
		time.Sleep(time.Second * 1)
	}
	if counter != 1 {
		t.Fatal("counter not 1")
	}
	w.Resume(id)
	time.Sleep(time.Second + time.Millisecond*100)
	w.Pause(id)
	fmt.Println("counter", counter)
	if counter != 2 {
		t.Fatal("counter not 2")
	}
}
