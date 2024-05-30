package main

import (
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func benchmarkLFQueue(b *testing.B, size int, numElements int) {

	numCores := runtime.NumCPU()
	runtime.GOMAXPROCS(numCores)
	for n := 0; n < b.N; n++ {
		queue := NewLFQueue(
			WithSize(size),
		)

		var wg sync.WaitGroup
		var elementsDequeued int64
		done := make(chan struct{})

		numProducers := 1
		elementsPerProducer := numElements / numProducers
		totalElements := int64(numElements)

		// Goroutines to enqueue elements
		wg.Add(numProducers)
		for i := 0; i < numProducers; i++ {
			go func(idx int) {
				// fmt.Println("Producer started", idx, elementsPerProducer)
				defer wg.Done()
				for j := 0; j < elementsPerProducer; j++ {
					// fmt.Println("Enqueued", j)
					queue.Enqueue(j)
				}
			}(i)
		}

		// Goroutine to dequeue elements
		wg.Add(1)
		go func() {
			defer wg.Done()
			for atomic.LoadInt64(&elementsDequeued) < totalElements {
				select {
				case data := <-queue.dataCh:
					atomic.AddInt64(&elementsDequeued, int64(len(data)))
					// fmt.Println("Dequeued", len(data))
				case <-done:
					return
				}
			}
			close(done)
		}()

		// Goroutine to trigger Tick every 1ms
		wg.Add(1)
		go func() {
			defer wg.Done()
			ticker := time.NewTicker(100 * time.Millisecond)
			defer ticker.Stop()
			for {
				select {
				case <-ticker.C:
					queue.Tick()
				case <-done:
					return
				}
			}
		}()

		// fmt.Println("Waiting for all goroutines to finish")
		// Wait for all goroutines to finish
		wg.Wait()
		// fmt.Println("Elements dequeued:", elementsDequeued)
		<-done
	}
}

func BenchmarkLFQueue1(b *testing.B) {
	benchmarkLFQueue(b, 128, 1)
}

func BenchmarkLFQueue10(b *testing.B) {
	benchmarkLFQueue(b, 128, 10)
}

func BenchmarkLFQueue100(b *testing.B) {
	benchmarkLFQueue(b, 128, 100)
}

func BenchmarkLFQueue1K(b *testing.B) {
	benchmarkLFQueue(b, 128*4, 1000)
}

func BenchmarkLFQueue10K(b *testing.B) {
	benchmarkLFQueue(b, 128*8*2, 10000)
}

func BenchmarkLFQueue100K(b *testing.B) {
	benchmarkLFQueue(b, 128*8*2, 100000)
}

func BenchmarkLFQueue1M(b *testing.B) {
	benchmarkLFQueue(b, 128*8*8, 1000000)
}

func BenchmarkLFQueue10M(b *testing.B) {
	benchmarkLFQueue(b, 128*8*8*2, 10000000)
}
