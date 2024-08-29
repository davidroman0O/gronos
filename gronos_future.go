package gronos

import (
	"context"
	"errors"
	"time"
)

/// After trying to implement a proper graph, I realized that we're not really managing the error cases when publishing messages properly.
/// To avoid having weird sync.Pool bugs due to concurrency, we're going to take a more pragmatic approach to own the results whatever it might be.

// type FuturePool[M any] struct {
// 	sync.Pool
// }

// func NewFuturePool[M any]() *FuturePool[M] {
// 	return &FuturePool[M]{
// 		Pool: sync.Pool{
// 			New: func() any {
// 				typeOf := reflect.TypeFor[M]().Elem()
// 				return reflect.New(typeOf).Interface()
// 			},
// 		},
// 	}
// }

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

type FutureResultOptions struct {
	Timeout *time.Duration
	Context context.Context
}

type FutureResultOption func(*FutureResultOptions)

func WithDefault() FutureResultOption {
	return func(o *FutureResultOptions) {
	}
}

func WithTimeout(timeout time.Duration) FutureResultOption {
	return func(o *FutureResultOptions) {
		o.Timeout = &timeout
	}
}

func WithContext(ctx context.Context) FutureResultOption {
	return func(o *FutureResultOptions) {
		o.Context = ctx
	}
}

func NewFutureResultOptions(opt FutureResultOption) FutureResultOptions {
	cfg := FutureResultOptions{}
	opt(&cfg)
	return cfg
}

func (m FutureMessage[T, R]) GetResult(opt FutureResultOption) Result {
	cfg := NewFutureResultOptions(opt)
	if cfg.Timeout != nil {
		return m.Result.WaitWithTimeout(*cfg.Timeout)
	}
	if cfg.Context != nil {
		return m.Result.Wait(cfg.Context)
	}
	return m.Result.Get()
}

// Secret sauce that allow that whole futureness to become a messaging system
type FutureMessageInterface interface {
	GetResult(opt FutureResultOption) Result
}

type Void struct{}

// Future represents an asynchronous computation that returns a Result
type Future[T any] chan Result

func (f Future[T]) Publish(value T) {
	f <- SuccessResult[T](value)
	close(f)
}

func (f Future[T]) PublishError(err error) {
	f <- FailureResult[T](err)
	close(f)
}

func (f Future[T]) Send(value T) {
	f <- SuccessResult[T](value)
}

func (f Future[T]) Error(err error) {
	f <- FailureResult[T](err)
}

func (f Future[T]) Close() {
	close(f)
}

func NewFuture[T any]() Future[T] {
	return make(Future[T], 1)
}

func NewFutureFailure[T any](err error) Future[T] {
	f := make(Future[T], 1)
	f <- Failure{Err: err}
	close(f)
	return f
}

// Get waits for the Future to complete and returns the Result
func (f Future[T]) Get() Result {
	return <-f
}

// Wait waits for the Future to complete or for the context to be cancelled
func (f Future[T]) Wait(ctx context.Context) Result {
	select {
	case res := <-f:
		return res
	case <-ctx.Done():
		return Failure{Err: errors.Join(ErrCancelled, ctx.Err())}
	}
}

// WaitWithTimeout waits for the Future to complete with a timeout
func (f Future[T]) WaitWithTimeout(timeout time.Duration) Result {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	switch value := f.Wait(ctx).(type) {
	case Failure:
		return Failure{Err: errors.Join(ErrTimeout, value.Err)}
	default:
		return value
	}
}
