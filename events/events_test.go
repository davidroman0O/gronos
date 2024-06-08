package events

import "testing"

type testKeyRegistry int

const (
	testKey1 testKeyRegistry = iota
	testKey2
	testKey3
)

type testMessage1 struct{}
type testMessage2 struct{}
type testMessage3 struct{}

func TestBasic(t *testing.T) {

	registry := New(
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

	_ = registry

}
