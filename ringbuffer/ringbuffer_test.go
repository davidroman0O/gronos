package ringbuffer

import (
	"fmt"
	"math/rand"
	"reflect"
	"sync/atomic"
	"testing"
	"time"

	"github.com/davidroman0O/gronos/clock"
)

func randomSleep(min, max time.Duration) {
	if max <= min {
		fmt.Println("max must be greater than min")
		return
	}
	rand.Seed(time.Now().UnixNano())
	delta := max - min
	sleepTime := time.Duration(rand.Int63n(int64(delta))) + min
	// fmt.Println("Sleeping for", sleepTime)
	time.Sleep(sleepTime)
}

func TestAdd(t *testing.T) {
	ck := clock.New(clock.WithInterval(1 * time.Second))
	rbInt := New[int](
		WithInitialSize(10),
		WithExpandable(true),
	)
	ck.Add(rbInt, clock.ManagedTimeline)

	go func() {
		for i := 0; i < 20; i++ {
			err := rbInt.Push(i)
			if err != nil {
				fmt.Println(err.Error())
				break
			}
			time.Sleep(100 * time.Millisecond)
		}
	}()

	go func() {
		for data := range rbInt.dataAvailable {
			fmt.Println("Data received:", data)
		}
	}()

	time.Sleep(3 * time.Second)
	rbInt.Close()
	ck.Stop()
}

func TestShared(t *testing.T) {
	ck := clock.New(clock.WithInterval(time.Second / 10))
	rbOne := New[int](
		WithInitialSize(10),
		WithExpandable(true),
	)
	rbTwo := New[int](
		WithInitialSize(10),
		WithExpandable(true),
	)

	ck.Add(rbOne, clock.ManagedTimeline)
	ck.Add(rbTwo, clock.ManagedTimeline)

	go func() {
		for i := 0; i < 10; i++ {
			err := rbOne.Push(i)
			if err != nil {
				fmt.Println(err.Error())
				break
			}
			randomSleep(time.Second/100, time.Second/50)
		}
		for i := 10; i < 20; i++ {
			err := rbOne.Push(i)
			if err != nil {
				fmt.Println(err.Error())
				break
			}
			randomSleep(time.Second/100, time.Second/50)
		}
	}()

	go func() {
		for i := 0; i < 10; i++ {
			err := rbTwo.Push(i)
			if err != nil {
				fmt.Println(err.Error())
				break
			}
			randomSleep(time.Second/100, time.Second/50)
		}
		for i := 10; i < 20; i++ {
			err := rbTwo.Push(i)
			if err != nil {
				fmt.Println(err.Error())
				break
			}
			randomSleep(time.Second/100, time.Second/50)
		}
	}()

	consumeOne := []int{}
	go func() {
		for data := range rbOne.dataAvailable {
			fmt.Println("Data one received:", data)
			consumeOne = append(consumeOne, data...)
		}
	}()

	consumeTwo := []int{}
	go func() {
		for data := range rbTwo.dataAvailable {
			fmt.Println("Data two received:", data)
			consumeTwo = append(consumeTwo, data...)
		}
	}()

	time.Sleep(3 * time.Second)
	rbOne.Close()
	ck.Stop()

	fmt.Println("consumeOne:", consumeOne)
	fmt.Println("consumeTwo:", consumeTwo)

	if reflect.DeepEqual(consumeOne, consumeTwo) {
		fmt.Println("consumeOne and consumeTwo are equal")
	} else {
		fmt.Println("consumeOne and consumeTwo are not equal")
	}
}

/////// Activation

func TestNewActiveRingBuffer(t *testing.T) {
	activationFunc := func(value int) bool {
		return value%2 == 0
	}

	rb := NewActivation[int](activationFunc, WithInitialSize(4), WithExpandable(true))

	if rb == nil {
		t.Fatalf("Expected ring buffer to be created, got nil")
	}
	if rb.size != 4 {
		t.Fatalf("Expected initial size to be 4, got %d", rb.size)
	}
	if rb.activation == nil {
		t.Fatalf("Expected activation function to be set, got nil")
	}
}

func TestActiveRingBufferPush(t *testing.T) {
	activationFunc := func(value int) bool {
		return value%2 == 0
	}

	rb := NewActivation[int](activationFunc, WithInitialSize(4))

	if err := rb.Push(1); err != nil {
		t.Fatalf("Expected push to succeed, got error: %v", err)
	}

	if err := rb.Push(2); err != nil {
		t.Fatalf("Expected push to succeed, got error: %v", err)
	}

	if atomic.LoadInt64(&rb.tail) != 2 {
		t.Fatalf("Expected tail to be 2, got %d", atomic.LoadInt64(&rb.tail))
	}
}

func TestActiveRingBufferTick(t *testing.T) {
	activationFunc := func(value int) bool {
		return value%2 == 0
	}

	rb := NewActivation[int](activationFunc, WithInitialSize(4))

	rb.Push(1)
	rb.Push(2)
	rb.Push(3)
	rb.Push(4)

	go rb.Tick()

	select {
	case data := <-rb.DataAvailable():
		if len(data) != 2 {
			t.Fatalf("Expected 2 elements in dataAvailable, got %d", len(data))
		}
		if data[0] != 2 || data[1] != 4 {
			t.Fatalf("Expected elements to be [2, 4], got %v", data)
		}
		fmt.Println(data)
	case <-time.After(time.Second):
		t.Fatalf("Expected data to be available, but got timeout")
	}

	if len(rb.buffer) == 4 {
		t.Fatalf("Expected buffer to be of size 0, got %d", len(rb.buffer))
	}
}

func TestActiveRingBufferResize(t *testing.T) {
	activationFunc := func(value int) bool {
		return value%2 == 0
	}

	rb := NewActivation[int](activationFunc, WithInitialSize(2), WithExpandable(true))

	rb.Push(1)
	rb.Push(2)
	rb.Push(3)
	rb.Push(4)

	go rb.Tick()

	select {
	case data := <-rb.DataAvailable():
		if len(data) != 2 {
			t.Fatalf("Expected 2 elements in dataAvailable, got %d", len(data))
		}
		if data[0] != 2 || data[1] != 4 {
			t.Fatalf("Expected elements to be [2, 4], got %v", data)
		}
	case <-time.After(time.Second):
		t.Fatalf("Expected data to be available, but got timeout")
	}

	if rb.size != 4 {
		t.Fatalf("Expected ring buffer size to be 4 after resize, got %d", rb.size)
	}
}

func TestActiveRingBufferClose(t *testing.T) {
	activationFunc := func(value int) bool {
		return value%2 == 0
	}

	rb := NewActivation[int](activationFunc, WithInitialSize(4))

	rb.Close()

	if rb.buffer != nil {
		t.Fatalf("Expected buffer to be nil after close, got %v", rb.buffer)
	}
	if rb.size != 0 {
		t.Fatalf("Expected size to be 0 after close, got %d", rb.size)
	}
	if rb.capacity != 0 {
		t.Fatalf("Expected capacity to be 0 after close, got %d", rb.capacity)
	}
}
