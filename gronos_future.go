package gronos

import (
	"context"
	"errors"
	"time"
)

/// After trying to implement a proper graph, I realized that we're not really managing the error cases when publishing messages properly.
/// To avoid having weird sync.Pool bugs due to concurrency, we're going to take a more pragmatic approach to own the results whatever it might be.

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

// Future represents an asynchronous computation that returns a Result
type Future[T any] chan Result

// Get waits for the Future to complete and returns the Result
func (f Future[T]) Get() Result {
	return <-f
}

// Wait waits for the Future to complete or for the context to be cancelled
func (f Future[T]) Wait(ctx context.Context) (Result, error) {
	select {
	case res := <-f:
		return res, nil
	case <-ctx.Done():
		return Failure{Err: ErrCancelled}, ctx.Err()
	}
}

// WaitWithTimeout waits for the Future to complete with a timeout
func (f Future[T]) WaitWithTimeout(timeout time.Duration) (Result, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	result, err := f.Wait(ctx)
	if err != nil {
		return Failure{Err: ErrTimeout}, err
	}
	return result, nil
}

// FutureVoid represents an asynchronous computation that doesn't return a value
type FutureVoid chan error

// Get waits for the FutureVoid to complete and returns the error (if any)
func (f FutureVoid) Get() error {
	return <-f
}

// Wait waits for the FutureVoid to complete or for the context to be cancelled
func (f FutureVoid) Wait(ctx context.Context) error {
	select {
	case err := <-f:
		return err
	case <-ctx.Done():
		return ErrCancelled
	}
}

// WaitWithTimeout waits for the FutureVoid to complete with a timeout
func (f FutureVoid) WaitWithTimeout(timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	err := f.Wait(ctx)
	if err == ErrCancelled {
		return ErrTimeout
	}
	return err
}
