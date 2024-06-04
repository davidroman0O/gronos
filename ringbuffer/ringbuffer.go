package ringbuffer

import (
	"errors"
	"sync/atomic"
)

/// After few experiments, I figured that `gronos` would need a special kind of api for its ring buffer. It is completely pointless to provide a direct and inspired actor-model approached to the messaging system since it would remove any determinism from the system. We want and should desire a time-based approach to allow batches of messages to accumulate and then be dispatched.
/// After using `gronos` for some tests, i realized that it was creating some unnessecary noise in the profiler.
/// We should have messages passed in batches to the ring buffer and then dispatched at a regular interval. This would allow for a more predictable system.
/// The developer can then chose what would be the timing and chose if it want to share it with other ring buffers.

// A ring buffer will do nothing until it is notified by a clock.
type RingBuffer[T any] struct {
	buffer        []T
	size          int
	capacity      int
	head          int64
	tail          int64
	expandable    bool
	dataAvailable chan []T
	stopCh        chan struct{}
	activation    func(T) bool
}

type ringBufferConfig struct {
	initialSize int
	expandable  bool
}

type ringBufferOption func(*ringBufferConfig)

func WithInitialSize(size int) ringBufferOption {
	return func(c *ringBufferConfig) {
		c.initialSize = size
	}
}

func WithExpandable(expandable bool) ringBufferOption {
	return func(c *ringBufferConfig) {
		c.expandable = expandable
	}
}

func New[T any](opts ...ringBufferOption) *RingBuffer[T] {
	c := ringBufferConfig{}
	for i := 0; i < len(opts); i++ {
		opts[i](&c)
	}
	rb := &RingBuffer[T]{
		buffer:        make([]T, c.initialSize),
		size:          c.initialSize,
		capacity:      c.initialSize,
		expandable:    c.expandable,
		dataAvailable: make(chan []T, 1),
		stopCh:        make(chan struct{}),
	}
	return rb
}

func NewActivation[T any](activation func(T) bool, opts ...ringBufferOption) *RingBuffer[T] {
	c := ringBufferConfig{}
	for i := 0; i < len(opts); i++ {
		opts[i](&c)
	}
	rb := &RingBuffer[T]{
		buffer:        make([]T, c.initialSize),
		size:          c.initialSize,
		capacity:      c.initialSize,
		expandable:    c.expandable,
		dataAvailable: make(chan []T, 1),
		stopCh:        make(chan struct{}),
		activation:    activation,
	}
	return rb
}

func (rb *RingBuffer[T]) DataAvailable() <-chan []T {
	return rb.dataAvailable
}

func (rb *RingBuffer[T]) Close() {
	close(rb.stopCh)
	rb.buffer = nil
	rb.size = 0
	rb.capacity = 0
	rb.head = 0
	rb.tail = 0
	close(rb.dataAvailable)
}

func (rb *RingBuffer[T]) Push(value T) error {
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

func (rb *RingBuffer[T]) resize() {
	newSize := 2 * rb.size
	newBuffer := make([]T, newSize)
	oldSize := rb.size

	for i := 0; i < oldSize; i++ {
		newBuffer[i] = rb.buffer[(atomic.LoadInt64(&rb.head)+int64(i))%int64(oldSize)]
	}

	rb.buffer = newBuffer
	rb.size = newSize
	atomic.StoreInt64(&rb.head, 0)
	atomic.StoreInt64(&rb.tail, int64(oldSize))
}

func (rb *RingBuffer[T]) Tick() {
	currentHead := atomic.LoadInt64(&rb.head)
	currentTail := atomic.LoadInt64(&rb.tail)
	if currentHead == currentTail {
		return // No data to send
	}

	size := int(currentTail - currentHead)
	tmpData := make([]T, size)

	// Copy data from buffer to tmpData while removing it from the buffer
	for i := 0; i < size; i++ {
		tmpData[i] = rb.buffer[currentHead%int64(rb.size)]
		currentHead++
	}
	atomic.StoreInt64(&rb.head, currentHead)

	activatedData := make([]T, 0, size)
	nonActivatedData := make([]T, 0, size)

	// Loop through tmpData, test each element with the activation function
	// and push it into the corresponding temporary array
	for _, element := range tmpData {
		if rb.activation == nil || rb.activation(element) {
			activatedData = append(activatedData, element)
		} else {
			nonActivatedData = append(nonActivatedData, element)
		}
	}

	// Push non-activated data back into the buffer
	for _, element := range nonActivatedData {
		err := rb.Push(element)
		if err != nil {
			// Handle ring buffer full error
			break
		}
	}

	if len(activatedData) == 0 {
		return // No activated data to send
	}

	select {
	case rb.dataAvailable <- activatedData:
	default:
		// If consumer is not ready, push activated data back into the buffer
		for _, element := range activatedData {
			err := rb.Push(element)
			if err != nil {
				// Handle ring buffer full error
				break
			}
		}
	}
}

var ErrRingBufferFull = errors.New("ring buffer is full")
