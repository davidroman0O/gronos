package nonos

import (
	"sync"
)

// ConcurrentArray represents a thread-safe array of comparable elements
type ConcurrentArray[T comparable] struct {
	data []T
	mu   sync.RWMutex
}

// New creates a new ConcurrentArray
func Array[T comparable]() *ConcurrentArray[T] {
	return &ConcurrentArray[T]{
		data: make([]T, 0),
	}
}

// Append adds an element to the array
func (ca *ConcurrentArray[T]) Append(item T) {
	ca.mu.Lock()
	defer ca.mu.Unlock()
	ca.data = append(ca.data, item)
}

// Get retrieves an element at a specific index
func (ca *ConcurrentArray[T]) Get(index int) (T, bool) {
	ca.mu.RLock()
	defer ca.mu.RUnlock()
	if index < 0 || index >= len(ca.data) {
		var zero T
		return zero, false
	}
	return ca.data[index], true
}

// Set updates an element at a specific index
func (ca *ConcurrentArray[T]) Set(index int, item T) bool {
	ca.mu.Lock()
	defer ca.mu.Unlock()
	if index < 0 || index >= len(ca.data) {
		return false
	}
	ca.data[index] = item
	return true
}

// Length returns the current length of the array
func (ca *ConcurrentArray[T]) Length() int {
	ca.mu.RLock()
	defer ca.mu.RUnlock()
	return len(ca.data)
}

// Contains checks if an item exists in the array
func (ca *ConcurrentArray[T]) Contains(item T) bool {
	ca.mu.RLock()
	defer ca.mu.RUnlock()
	for _, v := range ca.data {
		if v == item {
			return true
		}
	}
	return false
}
