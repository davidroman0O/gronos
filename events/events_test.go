package events

import (
	"testing"

	"github.com/davidroman0O/gronos/valueobjects"
)

type testMessage1 struct{}
type testMessage2 struct{}
type testMessage3 struct{}

func TestBasic(t *testing.T) {

	// The event loop must be able to define its own key type so the user can define all keys to get back it's own ports
	eventLoop, _ := New(
		WithPort(
			NewPort(
				valueobjects.PortKey("testKey1"),
				Register[testMessage1](),
				Register[testMessage2](),
				Register[testMessage3](),
			),
		),
		WithPort(
			NewPort(
				valueobjects.PortKey("testKey2"),
				Register[testMessage3](),
			),
		),
	)

	eventLoop.Tick()
}
