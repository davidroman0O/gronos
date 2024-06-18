package main

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// System holds a concurrent map of key/value pairs
type System struct {
	data sync.Map
}

// NewSystem creates a new System instance
func NewSystem() *System {
	return &System{}
}

// Set sets a key/value pair in the system
func (s *System) Set(key string, value interface{}) {
	s.data.Store(key, value)
}

// Get retrieves a value by key from the system
func (s *System) Get(key string) (interface{}, bool) {
	return s.data.Load(key)
}

// UsePort is an example of a utility function that interacts with the system
func (s *System) UsePort(ctx context.Context, key string, portKey string) (interface{}, error) {
	if val, ok := s.Get(key); ok {
		// Perform actions using the portKey and val
		return val, nil
	}
	return nil, fmt.Errorf("key not found: %s", key)
}

// contextKey is a type used for context keys to avoid collisions
type contextKey string

const systemContextKey = contextKey("system")

// WithSystem adds the System to the context
func WithSystem(ctx context.Context, system *System) context.Context {
	return context.WithValue(ctx, systemContextKey, system)
}

// GetSystem retrieves the System from the context
func GetSystem(ctx context.Context) (*System, bool) {
	system, ok := ctx.Value(systemContextKey).(*System)
	return system, ok
}

// RuntimeFunc is the type for user-provided functions that operate on the context
type RuntimeFunc func(ctx context.Context) error

// RunRuntimeFuncs runs runtime functions concurrently
func RunRuntimeFuncs(ctx context.Context, funcs []RuntimeFunc) error {
	var wg sync.WaitGroup
	errChan := make(chan error, len(funcs))

	for _, fn := range funcs {
		wg.Add(1)
		go func(f RuntimeFunc) {
			defer wg.Done()
			if err := f(ctx); err != nil {
				errChan <- err
			}
		}(fn)
	}

	wg.Wait()
	close(errChan)

	for err := range errChan {
		if err != nil {
			return err
		}
	}
	return nil
}

func main() {
	// Create a new system instance
	system := NewSystem()
	system.Set("key-runtime", "initial-runtime-data")
	system.Set("key-port-of-that-runtime", "initial-port-data")

	// Create a base context with the system
	ctx := WithSystem(context.Background(), system)

	// Define some runtime functions
	funcs := []RuntimeFunc{
		func(ctx context.Context) error {
			system, ok := GetSystem(ctx)
			if !ok {
				return fmt.Errorf("system not found in context")
			}
			system.Set("key-runtime", "updated-runtime-data")
			return nil
		},
		func(ctx context.Context) error {
			system, ok := GetSystem(ctx)
			if !ok {
				return fmt.Errorf("system not found in context")
			}
			time.Sleep(1 * time.Second)
			val, err := system.UsePort(ctx, "key-runtime", "key-port-of-that-runtime")
			if err != nil {
				return err
			}
			fmt.Println("Runtime 2:", val)
			return nil
		},
	}

	// Run the runtime functions
	if err := RunRuntimeFuncs(ctx, funcs); err != nil {
		fmt.Println("Error running runtime functions:", err)
	}
}
