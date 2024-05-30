package main

import (
	"fmt"
	"math/rand"
	"testing"
)

// Helper function to generate random values
func generateRandomValues(n int) []interface{} {
	values := make([]interface{}, n)
	for i := 0; i < n; i++ {
		values[i] = rand.Int()
	}
	return values
}

func BenchmarkEnqueue(b *testing.B) {
	for _, size := range []int{1, 10, 100, 1000, 10000, 100000, 1000000, 10000000} {
		b.Run(fmt.Sprintf("Size%d", size), func(b *testing.B) {
			q := NewLockFreeQueue()
			values := generateRandomValues(size)

			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				for _, value := range values {
					q.Enqueue(value)
				}
				// Trigger epoch increment to include memory reclamation overhead
				q.IncrementEpoch()
			}
		})
	}
}

func BenchmarkEnqueueN(b *testing.B) {
	for _, size := range []int{1, 10, 100, 1000, 10000, 100000, 1000000, 10000000} {
		b.Run(fmt.Sprintf("Size%d", size), func(b *testing.B) {
			q := NewLockFreeQueue()
			values := generateRandomValues(size)

			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				q.EnqueueN(values)
				// Trigger epoch increment to include memory reclamation overhead
				q.IncrementEpoch()
			}
		})
	}
}

func BenchmarkDequeue(b *testing.B) {
	for _, size := range []int{1, 10, 100, 1000, 10000, 100000, 1000000, 10000000} {
		b.Run(fmt.Sprintf("Size%d", size), func(b *testing.B) {
			q := NewLockFreeQueue()
			values := generateRandomValues(size)
			q.EnqueueN(values)
			threadID := 0

			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				for range values {
					q.Dequeue(threadID)
				}
				// Trigger epoch increment to include memory reclamation overhead
				q.IncrementEpoch()
			}
		})
	}
}

func BenchmarkDequeueN(b *testing.B) {
	for _, size := range []int{1, 10, 100, 1000, 10000, 100000, 1000000, 10000000} {
		b.Run(fmt.Sprintf("Size%d", size), func(b *testing.B) {
			q := NewLockFreeQueue()
			values := generateRandomValues(size)
			q.EnqueueN(values)
			threadID := 0

			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				q.DequeueN(threadID, size)
				// Trigger epoch increment to include memory reclamation overhead
				q.IncrementEpoch()
			}
		})
	}
}

func BenchmarkEpochReclamation(b *testing.B) {
	q := NewLockFreeQueue()
	values := generateRandomValues(100000)
	threadID := 0

	// Enqueue elements
	q.EnqueueN(values)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Dequeue elements to retire nodes
		q.DequeueN(threadID, 100000)
		// Increment epoch to trigger reclamation
		q.IncrementEpoch()
		q.IncrementEpoch()
		q.IncrementEpoch()
	}
}
