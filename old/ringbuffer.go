package gronos

import (
	"errors"
	"sync/atomic"
	"time"
)

/// TODO there is room for improvement, i'm pretty sure i can get 30M messages per seconds with some optimizations
/// TODO maybe keep `(atomic.LoadInt64(&rb.tail) - atomic.LoadInt64(&rb.head)) >= int64(rb.throughput)` but with a new condition with a timeout to release the data if the buffer is not filled in time

type ringBuffer[T any] struct {
	buffer        []T
	size          int
	capacity      int
	head          int64
	tail          int64
	expandable    bool
	dataAvailable chan []T
	ticker        *time.Ticker
	stopCh        chan struct{}
}

func newRingBuffer[T any](initialSize int, expandable bool, interval time.Duration) *ringBuffer[T] {
	rb := &ringBuffer[T]{
		buffer:        make([]T, initialSize),
		size:          initialSize,
		capacity:      initialSize,
		expandable:    expandable,
		dataAvailable: make(chan []T, 1),
		stopCh:        make(chan struct{}),
	}

	rb.ticker = time.NewTicker(interval)
	go rb.sendAvailableData()

	return rb
}

func (rb *ringBuffer[T]) Close() {
	rb.ticker.Stop()
	close(rb.stopCh)
	rb.buffer = nil
	rb.size = 0
	rb.capacity = 0
	rb.head = 0
	rb.tail = 0
	close(rb.dataAvailable)
}

func (rb *ringBuffer[T]) Push(value T) error {
	currentTail := atomic.LoadInt64(&rb.tail)
	nextTail := currentTail + 1
	if nextTail-atomic.LoadInt64(&rb.head) > int64(rb.size) {
		if rb.expandable {
			rb.resize()
		} else {
			return ErrRingBufferFull
		}
	}

	rb.buffer[currentTail%int64(rb.size)] = value
	atomic.StoreInt64(&rb.tail, nextTail)

	return nil
}

func (rb *ringBuffer[T]) PushN(values []T) error {
	for _, value := range values {
		if err := rb.Push(value); err != nil {
			return err
		}
	}
	return nil
}

func (rb *ringBuffer[T]) GetDataAvailableChannel() <-chan []T {
	return rb.dataAvailable
}

func (rb *ringBuffer[T]) sendAvailableData() {
	for {
		select {
		case <-rb.ticker.C:
			rb.notifyConsumer()
		case <-rb.stopCh:
			return
		}
	}
}

func (rb *ringBuffer[T]) notifyConsumer() {
	currentHead := atomic.LoadInt64(&rb.head)
	currentTail := atomic.LoadInt64(&rb.tail)
	if currentHead == currentTail {
		return // No data to send
	}

	size := int(currentTail - currentHead)
	data := make([]T, size)
	for i := 0; i < size; i++ {
		data[i] = rb.buffer[(currentHead+int64(i))%int64(rb.size)]
	}

	select {
	case rb.dataAvailable <- data:
		// Move head forward after successful send
		atomic.AddInt64(&rb.head, int64(size))
	default:
		// Non-blocking send; if the consumer is not ready, we skip.
	}
}

func (rb *ringBuffer[T]) resize() {
	newSize := 2 * rb.size
	newBuffer := make([]T, newSize)
	oldSize := rb.size

	// Properly transfer elements considering wrap around at the buffer end
	for i := 0; i < oldSize; i++ {
		newBuffer[i] = rb.buffer[(atomic.LoadInt64(&rb.head)+int64(i))%int64(oldSize)]
	}

	// Update buffer references
	rb.buffer = newBuffer
	rb.size = newSize

	// Reset head and tail relative to new buffer
	atomic.StoreInt64(&rb.head, 0)
	atomic.StoreInt64(&rb.tail, int64(oldSize))
}

func (rb *ringBuffer[T]) IsFull() bool {
	return (atomic.LoadInt64(&rb.tail) - atomic.LoadInt64(&rb.head)) >= int64(rb.size)
}

func (rb *ringBuffer[T]) IsEmpty() bool {
	return atomic.LoadInt64(&rb.head) == atomic.LoadInt64(&rb.tail)
}

func (rb *ringBuffer[T]) IsHigherPercentage(percentage int) bool {
	currentUsage := float64(atomic.LoadInt64(&rb.tail) - atomic.LoadInt64(&rb.head))
	currentCapacity := float64(rb.size)
	return (currentUsage / currentCapacity) > float64(percentage)/100
}

func (rb *ringBuffer[T]) IsLowerPercentage(percentage int) bool {
	currentUsage := float64(atomic.LoadInt64(&rb.tail) - atomic.LoadInt64(&rb.head))
	currentCapacity := float64(rb.size)
	return (currentUsage / currentCapacity) < float64(percentage)/100
}

var ErrRingBufferFull = errors.New("ring buffer is full")
