package gronos

import (
	"runtime"
	"sync/atomic"
	"unsafe"
)

const (
	minChunkSize      = 64
	defaultThroughput = 300
	messageBatchSize  = 1024 * 4
)

const (
	idle int32 = iota
	running
	stopped
)

type node[T any] struct {
	data []T
	next unsafe.Pointer
}

type ringBuffer[T any] struct {
	head, tail    *node[T]
	size          int64
	capacity      int64
	chunkSize     int64
	readerCount   int64
	writerCount   int64
	nodesInFlight int64
	procStatus    int32
	scheduler     Scheduler
	cb            ringCallback[T]
}

type ringCallback[T any] func([]T) bool

func newRingBuffer[T any](chunkSize int64, cb ringCallback[T]) *ringBuffer[T] {
	if chunkSize < minChunkSize {
		chunkSize = minChunkSize
	}
	head := &node[T]{
		data: make([]T, chunkSize),
	}
	head.next = unsafe.Pointer(head)
	return &ringBuffer[T]{
		head:      head,
		tail:      head,
		chunkSize: chunkSize,
		scheduler: NewScheduler(defaultThroughput),
		cb:        cb,
	}
}

func (rb *ringBuffer[T]) Push(data T) (int, bool) {
	return rb.pushN([]T{data})
}

func (rb *ringBuffer[T]) PushN(data []T) (int, bool) {
	return rb.pushN(data)
}

func (rb *ringBuffer[T]) pushN(data []T) (int, bool) {
	var start, end int
	for {
		if len(data) == 0 {
			return end, true
		}

		writerCount := atomic.LoadInt64(&rb.writerCount)
		readerCount := atomic.LoadInt64(&rb.readerCount)
		if writerCount-readerCount >= rb.capacity {
			// Ring buffer is full
			return end, false
		}

		currentTail := rb.tail
		nextNode := (*node[T])(atomic.LoadPointer((&currentTail.next)))

		count := int64(len(nextNode.data)) - (writerCount % rb.chunkSize)
		if count > int64(len(data)) {
			count = int64(len(data))
		}

		if atomic.CompareAndSwapPointer(&currentTail.next, unsafe.Pointer(nextNode), unsafe.Pointer(nextNode)) {
			copy(nextNode.data[writerCount%rb.chunkSize:], data[:count])
			atomic.AddInt64(&rb.writerCount, count)
			start += copy(data, data[:count])
			data = data[count:]
			if start == len(data) {
				rb.schedule()
				return start, true
			}
		}
	}
}

func (rb *ringBuffer[T]) Pop() (T, bool) {
	data, ok := rb.popN(1)
	if !ok {
		var zero T
		return zero, false
	}
	return data[0], true
}

func (rb *ringBuffer[T]) PopN(n int) ([]T, bool) {
	return rb.popN(n)
}

func (rb *ringBuffer[T]) popN(n int) ([]T, bool) {
	var data []T
	for {
		readerCount := atomic.LoadInt64(&rb.readerCount)
		writerCount := atomic.LoadInt64(&rb.writerCount)
		if readerCount == writerCount {
			// Ring buffer is empty
			return nil, false
		}

		currentHead := rb.head
		nextNode := (*node[T])(atomic.LoadPointer((&currentHead.next)))

		count := int64(len(nextNode.data)) - (readerCount % rb.chunkSize)
		if count > int64(n) {
			count = int64(n)
		}

		if atomic.CompareAndSwapPointer(&currentHead.next, unsafe.Pointer(nextNode), unsafe.Pointer(nextNode.next)) {
			data = append(data, nextNode.data[readerCount%rb.chunkSize:][:count]...)
			atomic.AddInt64(&rb.readerCount, count)
			n -= int(count)
			if n == 0 {
				return data, true
			}
		}
	}
}

func (rb *ringBuffer[T]) schedule() {
	if atomic.CompareAndSwapInt32(&rb.procStatus, idle, running) {
		rb.scheduler.Schedule(rb.process)
	}
}

func (rb *ringBuffer[T]) process() {
	rb.run()
	atomic.StoreInt32(&rb.procStatus, idle)
}

func (rb *ringBuffer[T]) run() {
	i, t := 0, rb.scheduler.Throughput()
	for atomic.LoadInt32(&rb.procStatus) != stopped {
		if i > t {
			i = 0
			runtime.Gosched()
		}
		i++

		if msgs, ok := rb.PopN(messageBatchSize); ok && len(msgs) > 0 {
			if rb.cb(msgs) {
				continue
			} else {
				// what to do bro?
			}
		} else {
			return
		}
	}
}

type Scheduler interface {
	Schedule(fn func())
	Throughput() int
}

type goscheduler int

func (goscheduler) Schedule(fn func()) {
	go fn()
}

func (sched goscheduler) Throughput() int {
	return int(sched)
}

func NewScheduler(throughput int) Scheduler {
	return goscheduler(throughput)
}
