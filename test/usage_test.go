package test

import (
	"context"
	"testing"
	"time"

	"github.com/davidroman0O/gronos/clock"
	"github.com/davidroman0O/gronos/events"
	"github.com/davidroman0O/gronos/valueobjects"
)

func testRuntimeA(ctx context.Context) error {
	return nil
}

func testRuntimeB(ctx context.Context) error {
	return nil
}

type messageA struct {
	msg string
}

// Let's try to compose gronos with all the parts we need
func TestUsage(t *testing.T) {

	c := clock.New(
		clock.WithName("test"),
		clock.WithInterval(time.Millisecond*100),
	)

	runtimeAKey := valueobjects.PortKey("runtimeA")

	loop, err := events.New(
		events.WithClock(c),
		events.WithPort(
			events.NewPort(
				runtimeAKey,
				events.Register[messageA](),
			),
		),
	)
	if err != nil {
		t.Fatal(err)
	}
	defer loop.Close()

	port, ok := loop.Load(runtimeAKey)
	if !ok {
		t.Fatal("port not found")
	}

	// a message with no metadata will just die
	if err := port.Push(
		context.Background(),
		valueobjects.NewMetadata(
			valueobjects.WithTo(runtimeAKey),
		),
		messageA{msg: "hello"}); err != nil {
		t.Fatal(err)
	}

}
