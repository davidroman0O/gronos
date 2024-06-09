package main

import (
	"fmt"
	"sync/atomic"
	"unsafe"
)

// Node represents a single node in the queue.
type Node struct {
	value interface{}
	next  *Node
}

// LockFreeQueue represents a lock-free queue.
type LockFreeQueue struct {
	head *Node
	tail *Node
	size int64
}

// NewLockFreeQueue creates a new lock-free queue.
func NewLockFreeQueue() *LockFreeQueue {
	dummy := &Node{}
	return &LockFreeQueue{
		head: dummy,
		tail: dummy,
		size: 0,
	}
}

// Enqueue adds an item to the queue.
func (q *LockFreeQueue) Enqueue(value interface{}) {
	newNode := &Node{value: value}
	for {
		tail := atomic.LoadPointer((*unsafe.Pointer)(unsafe.Pointer(&q.tail)))
		next := atomic.LoadPointer((*unsafe.Pointer)(unsafe.Pointer(&(*Node)(tail).next)))
		if tail == atomic.LoadPointer((*unsafe.Pointer)(unsafe.Pointer(&q.tail))) {
			if next == nil {
				if atomic.CompareAndSwapPointer(
					(*unsafe.Pointer)(unsafe.Pointer(&(*Node)(tail).next)),
					next,
					unsafe.Pointer(newNode),
				) {
					atomic.CompareAndSwapPointer(
						(*unsafe.Pointer)(unsafe.Pointer(&q.tail)),
						tail,
						unsafe.Pointer(newNode),
					)
					atomic.AddInt64(&q.size, 1)
					return
				}
			} else {
				atomic.CompareAndSwapPointer(
					(*unsafe.Pointer)(unsafe.Pointer(&q.tail)),
					tail,
					next,
				)
			}
		}
	}
}

// EnqueueN adds multiple items to the queue and returns the total enqueued and new length.
func (q *LockFreeQueue) EnqueueN(values []interface{}) (int, int64) {
	total := len(values)
	if total == 0 {
		return 0, atomic.LoadInt64(&q.size)
	}

	// Create nodes for all values
	nodes := make([]*Node, total)
	for i, value := range values {
		nodes[i] = &Node{value: value}
	}

	for {
		tail := atomic.LoadPointer((*unsafe.Pointer)(unsafe.Pointer(&q.tail)))
		next := atomic.LoadPointer((*unsafe.Pointer)(unsafe.Pointer(&(*Node)(tail).next)))
		if tail == atomic.LoadPointer((*unsafe.Pointer)(unsafe.Pointer(&q.tail))) {
			if next == nil {
				if atomic.CompareAndSwapPointer(
					(*unsafe.Pointer)(unsafe.Pointer(&(*Node)(tail).next)),
					next,
					unsafe.Pointer(nodes[0]),
				) {
					// Link the nodes together
					for i := 1; i < total; i++ {
						atomic.StorePointer((*unsafe.Pointer)(unsafe.Pointer(&nodes[i-1].next)), unsafe.Pointer(nodes[i]))
					}
					// Update tail pointer
					atomic.CompareAndSwapPointer(
						(*unsafe.Pointer)(unsafe.Pointer(&q.tail)),
						tail,
						unsafe.Pointer(nodes[total-1]),
					)
					atomic.AddInt64(&q.size, int64(total))
					return total, atomic.LoadInt64(&q.size)
				}
			} else {
				atomic.CompareAndSwapPointer(
					(*unsafe.Pointer)(unsafe.Pointer(&q.tail)),
					tail,
					next,
				)
			}
		}
	}
}

// Dequeue removes and returns an item from the queue.
func (q *LockFreeQueue) Dequeue() (interface{}, bool) {
	for {
		head := atomic.LoadPointer((*unsafe.Pointer)(unsafe.Pointer(&q.head)))
		tail := atomic.LoadPointer((*unsafe.Pointer)(unsafe.Pointer(&q.tail)))
		next := atomic.LoadPointer((*unsafe.Pointer)(unsafe.Pointer(&(*Node)(head).next)))
		if head == atomic.LoadPointer((*unsafe.Pointer)(unsafe.Pointer(&q.head))) {
			if head == tail {
				if next == nil {
					return nil, false
				}
				atomic.CompareAndSwapPointer(
					(*unsafe.Pointer)(unsafe.Pointer(&q.tail)),
					tail,
					next,
				)
			} else {
				value := (*Node)(next).value
				if atomic.CompareAndSwapPointer(
					(*unsafe.Pointer)(unsafe.Pointer(&q.head)),
					head,
					next,
				) {
					atomic.AddInt64(&q.size, -1)
					return value, true
				}
			}
		}
	}
}

// DequeueN removes up to n items from the queue and returns them along with the new length.
func (q *LockFreeQueue) DequeueN(n int) ([]interface{}, int64) {
	if n <= 0 {
		return nil, atomic.LoadInt64(&q.size)
	}

	items := make([]interface{}, 0, n)
	for {
		head := atomic.LoadPointer((*unsafe.Pointer)(unsafe.Pointer(&q.head)))
		tail := atomic.LoadPointer((*unsafe.Pointer)(unsafe.Pointer(&q.tail)))
		next := atomic.LoadPointer((*unsafe.Pointer)(unsafe.Pointer(&(*Node)(head).next)))

		if head == atomic.LoadPointer((*unsafe.Pointer)(unsafe.Pointer(&q.head))) {
			if head == tail {
				if next == nil {
					return items, atomic.LoadInt64(&q.size)
				}
				atomic.CompareAndSwapPointer(
					(*unsafe.Pointer)(unsafe.Pointer(&q.tail)),
					tail,
					next,
				)
			} else {
				for len(items) < n {
					if next == nil {
						break
					}
					items = append(items, (*Node)(next).value)
					head = next
					next = atomic.LoadPointer((*unsafe.Pointer)(unsafe.Pointer(&(*Node)(next).next)))
				}

				if atomic.CompareAndSwapPointer(
					(*unsafe.Pointer)(unsafe.Pointer(&q.head)),
					atomic.LoadPointer((*unsafe.Pointer)(unsafe.Pointer(&q.head))),
					head,
				) {
					atomic.AddInt64(&q.size, -int64(len(items)))
					return items, atomic.LoadInt64(&q.size)
				}
			}
		}
	}
}

// Quantity returns the number of items in the queue.
func (q *LockFreeQueue) Quantity() int64 {
	return atomic.LoadInt64(&q.size)
}

func main() {
	q := NewLockFreeQueue()

	// Enqueue elements
	q.Enqueue(1)
	q.Enqueue(2)
	q.Enqueue(3)

	// Check quantity
	fmt.Println("Quantity:", q.Quantity())

	// Enqueue batch elements
	total, newLen := q.EnqueueN([]interface{}{4, 5, 6})
	fmt.Printf("Enqueued %d elements, new length: %d\n", total, newLen)

	// Dequeue elements
	for i := 0; i < 3; i++ {
		value, ok := q.Dequeue()
		if ok {
			fmt.Println(value)
		} else {
			fmt.Println("Queue is empty")
		}
	}

	// Check quantity
	fmt.Println("Quantity:", q.Quantity())

	// Dequeue batch elements
	items, newLen := q.DequeueN(5)
	fmt.Printf("Dequeued elements: %v, new length: %d\n", items, newLen)

	// Check quantity
	fmt.Println("Quantity:", q.Quantity())
}
