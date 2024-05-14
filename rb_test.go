package gronos

import (
	"fmt"
	"math/rand"
	"reflect"
	"testing"
	"time"
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
	clock := NewClock(1 * time.Second)
	rbInt := NewRingBuffer[int](
		WithInitialSize(10),
		WithExpandable(true),
	)
	clock.Add(rbInt, ManagedTimeline)

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
	clock.Stop()
}

func TestShared(t *testing.T) {
	clock := NewClock(time.Second / 10)
	rbOne := NewRingBuffer[int](
		WithInitialSize(10),
		WithExpandable(true),
	)
	rbTwo := NewRingBuffer[int](
		WithInitialSize(10),
		WithExpandable(true),
	)

	clock.Add(rbOne, ManagedTimeline)
	clock.Add(rbTwo, ManagedTimeline)

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
	clock.Stop()

	fmt.Println("consumeOne:", consumeOne)
	fmt.Println("consumeTwo:", consumeTwo)

	if reflect.DeepEqual(consumeOne, consumeTwo) {
		fmt.Println("consumeOne and consumeTwo are equal")
	} else {
		fmt.Println("consumeOne and consumeTwo are not equal")
	}
}
