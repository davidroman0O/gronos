package main

import (
	"fmt"
	"sync"
	"sync/atomic"
)

type Node struct {
	data     []interface{}
	next     *Node
	capacity int
	size     int64
}

type LFQueue struct {
	head             *Node
	tail             *Node
	dataCh           chan []interface{}
	activation       func(interface{}) bool
	nodeSize         int
	availableNodes   int64
	mutex            sync.Mutex
	activationExists bool
}

type Option func(*LFQueue)

func WithSize(size int) Option {
	return func(q *LFQueue) {
		q.nodeSize = size
	}
}

func NewLFQueue(opts ...Option) *LFQueue {
	queue := &LFQueue{
		nodeSize: 128,                         // default value
		dataCh:   make(chan []interface{}, 1), // buffer size of 1 for batching
	}
	for _, opt := range opts {
		opt(queue)
	}
	initialNode := &Node{
		data:     make([]interface{}, 0, queue.nodeSize),
		capacity: queue.nodeSize,
	}
	queue.head = initialNode
	queue.tail = initialNode
	return queue
}

func NewActivateLFQueue(activation func(interface{}) bool, opts ...Option) *LFQueue {
	queue := NewLFQueue(opts...)
	queue.activation = activation
	queue.activationExists = true
	return queue
}

func (q *LFQueue) Enqueue(item interface{}) {
	q.enqueue(item)
}

func (q *LFQueue) EnqueueN(items []interface{}) {
	for _, item := range items {
		q.enqueue(item)
	}
}

func (q *LFQueue) enqueue(item interface{}) {
	if atomic.LoadInt64(&q.availableNodes) == 1 {
		q.mutex.Lock()
		defer q.mutex.Unlock()
	}

	node := q.tail
	for atomic.LoadInt64(&node.size) >= int64(node.capacity) {
		newNode := &Node{
			data:     make([]interface{}, 0, q.nodeSize),
			capacity: q.nodeSize,
		}
		node.next = newNode
		node = newNode
		atomic.StoreInt64(&node.size, 0)
		q.tail = node
		atomic.AddInt64(&q.availableNodes, 1)
	}

	node.data = append(node.data, item)
	atomic.AddInt64(&node.size, 1)
}

func (q *LFQueue) Tick() {
	if q.activationExists {
		q.tickWithActivation()
	} else {
		q.tickWithoutActivation()
	}
}

func (q *LFQueue) tickWithActivation() {
	node := q.head
	if atomic.LoadInt64(&node.size) == 0 {
		return
	}

	var activated []interface{}
	var nonActivated []interface{}

	for _, item := range node.data {
		if q.activation(item) {
			activated = append(activated, item)
		} else {
			nonActivated = append(nonActivated, item)
		}
	}

	q.dataCh <- activated
	if len(nonActivated) > 0 {
		q.EnqueueN(nonActivated)
	}

	q.recycleNode(node)
}

func (q *LFQueue) tickWithoutActivation() {
	node := q.head
	if atomic.LoadInt64(&node.size) == 0 {
		return
	}

	// Retrieve and empty the node
	data := make([]interface{}, len(node.data))
	copy(data, node.data)
	q.dataCh <- data

	q.recycleNode(node)
}

func (q *LFQueue) recycleNode(node *Node) {
	if node.next != nil {
		q.head = node.next
	} else {
		node.data = node.data[:0]
		q.head = node
	}
	atomic.AddInt64(&q.availableNodes, -1)
	atomic.StoreInt64(&node.size, 0)
}

func main() {
	// Example usage
	queue := NewLFQueue()

	go func() {
		for {
			data := <-queue.dataCh
			fmt.Println(len(data))
		}
	}()

	counter := 0
	for range 128 * 5 {
		counter++
		queue.Enqueue(counter)
	}
	queue.Tick()
	for range 128 * 5 {
		counter++
		queue.Enqueue(counter)
	}
	queue.Tick()
	fmt.Println("")
	fmt.Println(atomic.LoadInt64(&queue.availableNodes))
}
