package gronos

import (
	"testing"
	"time"
)

func TestRouter(t *testing.T) {
	g, err := New()
	if err != nil {
		t.Errorf("Error creating new context: %v", err)
	}

	id, _ := g.Add(
		"simple",
		WithRuntime(testRuntime),
		WithTimeout(time.Second*5),
		WithValue("testRuntime", "testRuntime"))

	r := newRouter()

	// we will remove the g.runtimes soon
	station, _ := g.runtimes.Get(id)
	r.Add(*station)

	r.Close()
}
