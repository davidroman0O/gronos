package main

import (
	"context"
	"fmt"
	"time"
)

// MaybeResult represents a result that may or may not have a value, or may have an error
type MaybeResult[T any] struct {
	Value T
	Err   error
	Valid bool
}

// Just creates a successful MaybeResult with a value
func Just[T any](value T) MaybeResult[T] {
	return MaybeResult[T]{Value: value, Valid: true}
}

// Err creates an error MaybeResult
func Err[T any](err error) MaybeResult[T] {
	return MaybeResult[T]{Err: err, Valid: true}
}

// Nothing creates a MaybeResult with no value
func Nothing[T any]() MaybeResult[T] {
	return MaybeResult[T]{Valid: false}
}

// IsJust returns true if the MaybeResult has a valid value
func (m MaybeResult[T]) IsJust() bool {
	return m.Valid && m.Err == nil
}

// IsErr returns true if the MaybeResult has an error
func (m MaybeResult[T]) IsErr() bool {
	return m.Valid && m.Err != nil
}

// IsNothing returns true if the MaybeResult has no value
func (m MaybeResult[T]) IsNothing() bool {
	return !m.Valid
}

// Future represents an asynchronous computation that returns a value
type Future[T any] chan MaybeResult[T]

// FutureVoid represents an asynchronous computation that doesn't return a value
type FutureVoid chan error

// Wait waits for the Future to complete or for the context to be cancelled
func (f Future[T]) Wait(ctx context.Context) (MaybeResult[T], error) {
	select {
	case res := <-f:
		return res, nil
	case <-ctx.Done():
		return Nothing[T](), ctx.Err()
	}
}

// WaitWithTimeout waits for the Future to complete with a timeout
func (f Future[T]) WaitWithTimeout(timeout time.Duration) (MaybeResult[T], error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	return f.Wait(ctx)
}

// Wait waits for the FutureVoid to complete or for the context to be cancelled
func (f FutureVoid) Wait(ctx context.Context) error {
	select {
	case err := <-f:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

// WaitWithTimeout waits for the FutureVoid to complete with a timeout
func (f FutureVoid) WaitWithTimeout(timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	return f.Wait(ctx)
}

func main() {
	comValue := make(chan Future[int])
	comVoid := make(chan FutureVoid)

	go func() {
		ticker := time.NewTicker(1 * time.Second)
		for i := 0; ; i++ {
			select {
			case <-ticker.C:
				if i%2 == 0 {
					future := make(Future[int], 1)
					if i%4 == 0 {
						future <- Just(i)
					} else {
						future <- Err[int](fmt.Errorf("example error"))
					}
					comValue <- future
				} else {
					futureVoid := make(FutureVoid, 1)
					if i%4 == 1 {
						futureVoid <- nil // Success
					} else {
						futureVoid <- fmt.Errorf("void error")
					}
					comVoid <- futureVoid
				}
			}
		}
	}()

	for i := 0; i < 10; i++ {
		select {
		case v := <-comValue:
			res, err := v.WaitWithTimeout(2 * time.Second)
			if err != nil {
				fmt.Printf("Future timeout or context error: %v\n", err)
			} else if res.IsJust() {
				fmt.Printf("Future value: %d\n", res.Value)
			} else if res.IsErr() {
				fmt.Printf("Future error: %v\n", res.Err)
			} else {
				fmt.Println("Future: No value (Nothing)")
			}

		case v := <-comVoid:
			err := v.WaitWithTimeout(2 * time.Second)
			if err != nil {
				fmt.Printf("FutureVoid error or timeout: %v\n", err)
			} else {
				fmt.Println("FutureVoid: Success")
			}
		}
	}
}
