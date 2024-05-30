package main

import (
	"testing"
)

// Unit tests
func TestEnqueue(t *testing.T) {
	q := NewLockFreeQueue()
	q.Enqueue(1)
	q.Enqueue(2)
	q.Enqueue(3)

	if got, _ := q.Dequeue(0); got != 1 {
		t.Errorf("expected 1, got %v", got)
	}
	if got, _ := q.Dequeue(0); got != 2 {
		t.Errorf("expected 2, got %v", got)
	}
	if got, _ := q.Dequeue(0); got != 3 {
		t.Errorf("expected 3, got %v", got)
	}
	if _, ok := q.Dequeue(0); ok {
		t.Errorf("expected queue to be empty")
	}
}

func TestDequeueEmpty(t *testing.T) {
	q := NewLockFreeQueue()
	if _, ok := q.Dequeue(0); ok {
		t.Errorf("expected queue to be empty")
	}
}

func TestEnqueueDequeueInterleaved(t *testing.T) {
	q := NewLockFreeQueue()
	q.Enqueue(1)
	if got, _ := q.Dequeue(0); got != 1 {
		t.Errorf("expected 1, got %v", got)
	}
	q.Enqueue(2)
	q.Enqueue(3)
	if got, _ := q.Dequeue(0); got != 2 {
		t.Errorf("expected 2, got %v", got)
	}
	q.Enqueue(4)
	if got, _ := q.Dequeue(0); got != 3 {
		t.Errorf("expected 3, got %v", got)
	}
	if got, _ := q.Dequeue(0); got != 4 {
		t.Errorf("expected 4, got %v", got)
	}
}

func TestRetireNode(t *testing.T) {
	q := NewLockFreeQueue()
	q.Enqueue(1)
	q.Enqueue(2)
	q.Enqueue(3)

	q.Dequeue(0)
	q.Dequeue(0)
	q.Dequeue(0)

	if len(q.epoch.retired[0]) != 3 {
		t.Errorf("expected 3 retired nodes, got %v", len(q.epoch.retired[0]))
	}
}

func TestEpochReclamation(t *testing.T) {
	q := NewLockFreeQueue()

	// Enqueue some elements
	q.Enqueue(1)
	q.Enqueue(2)
	q.Enqueue(3)

	// Dequeue all elements
	q.Dequeue(0)
	q.Dequeue(0)
	q.Dequeue(0)

	// Ensure nodes are retired
	if len(q.epoch.retired[0]) != 3 {
		t.Errorf("expected 3 retired nodes, got %v", len(q.epoch.retired[0]))
	}

	// Move epoch to trigger reclamation
	q.IncrementEpoch()
	q.IncrementEpoch()
	q.IncrementEpoch()

	// Ensure nodes are reclaimed
	if len(q.epoch.retired[0]) != 0 {
		t.Errorf("expected 0 retired nodes, got %v", len(q.epoch.retired[0]))
	}
}

func TestEnqueueN(t *testing.T) {
	q := NewLockFreeQueue()
	values := []interface{}{1, 2, 3, 4, 5}
	q.EnqueueN(values)

	for i := 0; i < len(values); i++ {
		if got, _ := q.Dequeue(0); got != values[i] {
			t.Errorf("expected %v, got %v", values[i], got)
		}
	}
}

func TestDequeueN(t *testing.T) {
	q := NewLockFreeQueue()
	values := []interface{}{1, 2, 3, 4, 5}
	q.EnqueueN(values)

	dequeuedValues, _ := q.DequeueN(0, 3)
	expectedValues := []interface{}{1, 2, 3}

	for i := 0; i < len(expectedValues); i++ {
		if dequeuedValues[i] != expectedValues[i] {
			t.Errorf("expected %v, got %v", expectedValues[i], dequeuedValues[i])
		}
	}

	dequeuedValues, _ = q.DequeueN(0, 3)
	expectedValues = []interface{}{4, 5}

	for i := 0; i < len(expectedValues); i++ {
		if dequeuedValues[i] != expectedValues[i] {
			t.Errorf("expected %v, got %v", expectedValues[i], dequeuedValues[i])
		}
	}
}
