package main

import (
	"fmt"
	"math/rand"
	"testing"
)

// Benchmark for single Enqueue operation
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
			}
		})
	}
}

// Benchmark for batch EnqueueN operation
func BenchmarkEnqueueN(b *testing.B) {
	for _, size := range []int{1, 10, 100, 1000, 10000, 100000, 1000000, 10000000} {
		b.Run(fmt.Sprintf("Size%d", size), func(b *testing.B) {
			q := NewLockFreeQueue()
			values := generateRandomValues(size)

			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				q.EnqueueN(values)
			}
		})
	}
}

// Benchmark for single Dequeue operation
func BenchmarkDequeue(b *testing.B) {
	for _, size := range []int{1, 10, 100, 1000, 10000, 100000, 1000000, 10000000} {
		b.Run(fmt.Sprintf("Size%d", size), func(b *testing.B) {
			q := NewLockFreeQueue()
			values := generateRandomValues(size)
			q.EnqueueN(values)

			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				for range values {
					q.Dequeue()
				}
			}
		})
	}
}

// Benchmark for batch DequeueN operation
func BenchmarkDequeueN(b *testing.B) {
	for _, size := range []int{1, 10, 100, 1000, 10000, 100000, 1000000, 10000000} {
		b.Run(fmt.Sprintf("Size%d", size), func(b *testing.B) {
			q := NewLockFreeQueue()
			values := generateRandomValues(size)
			q.EnqueueN(values)

			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				q.DequeueN(size)
			}
		})
	}
}

// Helper function to generate random values
func generateRandomValues(n int) []interface{} {
	values := make([]interface{}, n)
	for i := 0; i < n; i++ {
		values[i] = rand.Int()
	}
	return values
}
