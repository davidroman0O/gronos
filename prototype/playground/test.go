package main

import (
	"errors"
	"fmt"
	"reflect"
	"time"
)

// Common errors
var (
	ErrNoValue     = errors.New("no value present")
	ErrTimeout     = errors.New("operation timed out")
	ErrCancelled   = errors.New("operation was cancelled")
	ErrUnavailable = errors.New("resource unavailable")
)

// Result is an interface that represents the outcome of an operation
type Result interface {
	isResult()
}

// Success represents a successful operation with a value
type Success[T any] struct {
	Value T
}

func (Success[T]) isResult() {}

// Failure represents a failed operation with an error
type Failure struct {
	Err error
}

func (Failure) isResult() {}

// MaybeResult is a type alias for Result to maintain backwards compatibility
type MaybeResult[T any] Result

// SuccessResult creates a successful Result with a value
func SuccessResult[T any](value T) MaybeResult[T] {
	return Success[T]{Value: value}
}

// FailureResult creates a failed Result with an error
func FailureResult[T any](err error) MaybeResult[T] {
	return Failure{Err: err}
}

// Message is a generic type to hold both the data and the result
type FutureMessage[T any, R any] struct {
	Data   T
	Result Future[R]
}

func (m FutureMessage[T, R]) GetResult() Result {
	return <-m.Result
}

type FutureMessageVoid[T any] struct {
	Data   T
	Result FutureVoid
}

type Void struct{}

// Future represents an asynchronous computation that returns a Result
type Future[T any] chan Result

// FutureVoid represents an asynchronous computation that doesn't return a value
type FutureVoid chan Result

// Example message types
type MyMessage struct {
	Content string
}

type MyResponse struct {
	Status string
}

func main() {
	// Create a FutureMessage
	futureMsg := FutureMessage[MyMessage, MyResponse]{
		Data:   MyMessage{Content: "Hello, World!"},
		Result: make(Future[MyResponse]),
	}

	// Create a FutureMessageVoid
	futureMsgVoid := FutureMessageVoid[MyMessage]{
		Data:   MyMessage{Content: "Void message"},
		Result: make(FutureVoid),
	}

	// Simulate async operations
	go func() {
		time.Sleep(time.Second)
		futureMsg.Result <- SuccessResult(MyResponse{Status: "OK"})
	}()

	go func() {
		time.Sleep(time.Second)
		futureMsgVoid.Result <- SuccessResult(Void{})
	}()

	// Function to check message type and retrieve result
	checkAndRetrieveResult := func(msg interface{}) {
		msgType := reflect.TypeOf(msg)
		msgValue := reflect.ValueOf(msg)

		fmt.Printf("Message type: %v\n", msgType)

		if resultField := msgValue.FieldByName("Result"); resultField.IsValid() {
			fmt.Println("Message has a Result field")

			// Check if it's a FutureMessage or FutureMessageVoid
			if msgType.AssignableTo(reflect.TypeOf((*FutureMessage[any, any])(nil)).Elem()) {
				fmt.Println("It's a FutureMessage")
				switch value := msg.(FutureMessage[MyMessage, MyResponse]).GetResult().(type) {
				case Success[MyResponse]:
					fmt.Printf("Result: %+v\n", value)
				case Failure:
					fmt.Printf("Error: %v\n", value.Err)
				}
			} else if msgType.AssignableTo(reflect.TypeOf((*FutureMessageVoid[MyMessage])(nil)).Elem()) {
				fmt.Println("It's a FutureMessageVoid")
				switch value := (<-msg.(FutureMessageVoid[MyMessage]).Result).(type) {
				case Success[Void]:
					fmt.Println("Result: Success")
				case Failure:
					fmt.Printf("Error: %v\n", value.Err)
				}
			} else {
				fmt.Println("Unknown Future type")
			}
		} else {
			fmt.Println("Message does not have a Result field")
		}

		fmt.Println()
	}

	// Check and retrieve results
	checkAndRetrieveResult(futureMsg)
	checkAndRetrieveResult(futureMsgVoid)

	// Example of an unsupported type
	unsupportedMsg := MyMessage{Content: "Unsupported"}
	checkAndRetrieveResult(unsupportedMsg)

	// Wait for async operations to complete
	time.Sleep(2 * time.Second)

	// // Demonstrate usage of Wait and WaitWithTimeout
	// ctx := context.Background()
	// result, err := futureMsg.Result.Wait(ctx)
	// fmt.Printf("Wait result: %+v, error: %v\n", result, err)

	// result, err = futureMsg.Result.WaitWithTimeout(500 * time.Millisecond)
	// fmt.Printf("WaitWithTimeout result: %+v, error: %v\n", result, err)
}
