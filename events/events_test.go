package events

import "testing"

type testUserDefinedEventLoopKey int

const (
	testKey1 testUserDefinedEventLoopKey = iota
	testKey2
	testKey3
)

type testMessage1 struct{}
type testMessage2 struct{}
type testMessage3 struct{}

func TestBasic(t *testing.T) {

	// The event loop must be able to define its own key type so the user can define all keys to get back it's own ports
	eventLoop, _ := New[testUserDefinedEventLoopKey](
		NewPort(
			testKey1,
			Register[testMessage1](),
			Register[testMessage2](),
			Register[testMessage3](),
		),
		NewPort(
			testKey2,
			Register[testMessage3](),
		),
	)

	eventLoop.Tick()
}
