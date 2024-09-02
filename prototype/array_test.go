package gronos

import (
	"sync"
	"testing"
)

func TestConcurrentArray(t *testing.T) {
	t.Run("Basic operations", func(t *testing.T) {
		ca := Array[int]()

		// Test Append and Length
		ca.Append(1)
		ca.Append(2)
		ca.Append(3)

		if ca.Length() != 3 {
			t.Errorf("Expected length 3, got %d", ca.Length())
		}

		// Test Get
		val, ok := ca.Get(1)
		if !ok || val != 2 {
			t.Errorf("Expected value 2 at index 1, got %d", val)
		}

		// Test Set
		ca.Set(1, 5)
		val, _ = ca.Get(1)
		if val != 5 {
			t.Errorf("Expected value 5 at index 1 after Set, got %d", val)
		}

		// Test Contains
		if !ca.Contains(5) {
			t.Error("Expected Contains(5) to return true")
		}
		if ca.Contains(10) {
			t.Error("Expected Contains(10) to return false")
		}
	})

	t.Run("Concurrent operations", func(t *testing.T) {
		ca := Array[int]()
		const goroutines = 100
		const operationsPerGoroutine = 1000

		var wg sync.WaitGroup
		wg.Add(goroutines)

		for i := 0; i < goroutines; i++ {
			go func() {
				defer wg.Done()
				for j := 0; j < operationsPerGoroutine; j++ {
					ca.Append(j)
					_, _ = ca.Get(j % 10)
					ca.Set(j%10, j)
					_ = ca.Contains(j)
				}
			}()
		}

		wg.Wait()

		expectedLength := goroutines * operationsPerGoroutine
		if ca.Length() != expectedLength {
			t.Errorf("Expected length %d, got %d", expectedLength, ca.Length())
		}
	})

	t.Run("Edge cases", func(t *testing.T) {
		ca := Array[int]()

		// Test Get on empty array
		_, ok := ca.Get(0)
		if ok {
			t.Error("Expected Get on empty array to return false")
		}

		// Test Set on empty array
		ok = ca.Set(0, 1)
		if ok {
			t.Error("Expected Set on empty array to return false")
		}

		// Test Get with negative index
		_, ok = ca.Get(-1)
		if ok {
			t.Error("Expected Get with negative index to return false")
		}

		// Test Set with negative index
		ok = ca.Set(-1, 1)
		if ok {
			t.Error("Expected Set with negative index to return false")
		}

		// Test Get with out of bounds index
		ca.Append(1)
		_, ok = ca.Get(1)
		if ok {
			t.Error("Expected Get with out of bounds index to return false")
		}
	})
}
