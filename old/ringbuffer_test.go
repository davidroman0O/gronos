package gronos

import (
	"fmt"
	"sync"
	"testing"
	"time"
)

// TestSelectWithDataChannel tests the RingBuffer's interaction with a select statement.
func TestSelectWithDataChannel(t *testing.T) {
	rb := newRingBuffer[int](5, true, 3)
	totalItems := 6
	mockEventTriggered := false
	mockEvent := make(chan bool, 1) // Non-blocking channel for a mock event

	// Simulate sending data to the ring buffer
	go func() {
		for i := 0; i < totalItems; i++ {
			if err := rb.Push(i); err != nil {
				t.Errorf("Failed to push data: %v", err)
			}
			// Trigger mock event halfway through data push
			if i == totalItems/2 {
				mockEvent <- true
			}
		}
	}()

	receivedItems := 0
	timer := time.NewTimer(1 * time.Second)
	defer timer.Stop()

	for receivedItems < totalItems {
		select {
		case data := <-rb.GetDataAvailableChannel():
			fmt.Printf("Received data from RingBuffer: %v\n", data)
			receivedItems += len(data)

		case <-mockEvent:
			mockEventTriggered = true
			fmt.Println("Mock event triggered")

		case <-timer.C:
			if !mockEventTriggered {
				t.Errorf("Timeout occurred; mock event was not triggered")
			}
			if receivedItems < totalItems {
				t.Errorf("Timeout occurred; only %d items received out of %d", receivedItems, totalItems)
			}
			return
		}

		// Reset the timer as long as we are still expecting more data or events
		if !timer.Stop() {
			<-timer.C
		}
		timer.Reset(500 * time.Millisecond)
	}

	if receivedItems != totalItems {
		t.Errorf("Did not receive all expected items, expected: %d, got: %d", totalItems, receivedItems)
	}
	if !mockEventTriggered {
		t.Errorf("Mock event should have been triggered but was not")
	}
}

func TestExpandableRingBuffer(t *testing.T) {
	rb := newRingBuffer[int](2, true, 1) // small initial size to trigger expansion
	go func() {
		for i := 0; i < 10; i++ { // Push enough items to require expansion
			if err := rb.Push(i); err != nil {
				t.Errorf("Failed to push: %v", err)
			}
		}
	}()

	time.Sleep(1 * time.Second) // Allow time for operations to process
	if rb.size <= 2 {
		t.Errorf("RingBuffer did not expand as expected. Current size: %d", rb.size)
	}
}

func TestUnexpandableRingBuffer(t *testing.T) {
	rb := newRingBuffer[int](2, false, 1) // Capacity for 2 items, not expandable

	// Fill buffer to capacity
	if err := rb.Push(1); err != nil {
		t.Fatalf("Failed to push first item: %v", err)
	}
	if err := rb.Push(2); err != nil {
		t.Fatalf("Failed to push second item: %v", err)
	}

	// Attempt to push to a full buffer
	err := rb.Push(3)
	if err == nil {
		t.Fatalf("Expected an error when pushing to a full, unexpandable buffer but did not receive one")
	} else {
		t.Logf("Received expected error when pushing to full buffer: %v", err)
	}
}

// go test -v -timeout=10s -run ^BenchmarkRingBufferThroughput$  -bench=BenchmarkRingBufferThroughput -benchtime=20s
func BenchmarkRingBufferThroughput(b *testing.B) {
	rb := newRingBuffer[int](1024*8, true, 1024) // Large initial size to reduce resizing impact
	totalMessages := 1000 * 1                    // 10 million messages
	b.ResetTimer()

	var wg sync.WaitGroup
	wg.Add(2)

	// Producer goroutine
	go func() {
		defer wg.Done()
		for i := 0; i < totalMessages; i++ {
			if err := rb.Push(i); err != nil {
				b.Errorf("Failed to push data: %v", err)
				return
			}
		}
	}()

	// Consumer goroutine
	go func() {
		defer wg.Done()
		receivedCount := 0
		for receivedCount < totalMessages {
			select {
			case data := <-rb.GetDataAvailableChannel():
				receivedCount += len(data)
				fmt.Println(data)
			default:
				// Continuously try to read to keep up with the producer
			}
		}
	}()

	wg.Wait()
}

// go test -v -timeout=10s -run ^TestThroughput$
func TestThroughput(t *testing.T) {
	rb := newRingBuffer[int](1024*8, true, 1024*4) // Large initial size to reduce resizing impact
	totalMessages := 1000000 * 3                   // 10 million messages

	var wg sync.WaitGroup
	wg.Add(2)

	now := time.Now()

	// Producer goroutine
	go func() {
		defer wg.Done()
		// fmt.Println("starting push")
		for i := 0; i < totalMessages; i++ {
			if err := rb.Push(i); err != nil {
				t.Errorf("Failed to push data: %v", err)
				return
			}
		}
	}()

	// Consumer goroutine
	go func() {
		defer wg.Done()
		receivedCount := 0
		// fmt.Println("starting consume")
		for receivedCount < totalMessages {
			select {
			case data := <-rb.GetDataAvailableChannel():
				receivedCount += len(data)
				// fmt.Println(data)
				// fmt.Printf("*")
			default:
				// fmt.Printf("-")
				// Continuously try to read to keep up with the producer
			}
		}
	}()

	wg.Wait()
	fmt.Println("Total time taken: ", time.Since(now))
	// <-time.After(1 * time.Second)
}
