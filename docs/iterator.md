# Gronos Iterator

The Iterator functionality in Gronos allows you to create applications that execute a sequence of tasks in a continuous loop. This document covers the usage and configuration of Iterators in Gronos.

## Table of Contents

1. [Iterator Function](#iterator-function)
2. [CancellableTask](#cancellabletask)
3. [LoopableIterator](#loopableiterator)
4. [Creating an Iterator](#creating-an-iterator)
5. [Adding an Iterator to Gronos](#adding-an-iterator-to-gronos)
6. [Examples](#examples)

## Iterator Function

The `Iterator` function creates a `RuntimeApplication` that executes a sequence of tasks in a loop:

```go
func Iterator(iterCtx context.Context, tasks []CancellableTask, opts ...IteratorOption) RuntimeApplication
```

- `iterCtx`: A context for controlling the iterator's lifecycle.
- `tasks`: A slice of `CancellableTask` functions to be executed in sequence.
- `opts`: Optional configuration options for the iterator.

## CancellableTask

A `CancellableTask` is a function that can be cancelled via its context:

```go
type CancellableTask func(ctx context.Context) error
```

## LoopableIterator

The `LoopableIterator` is the underlying structure used by the `Iterator` function:

```go
type LoopableIterator struct {
    // ... (internal fields)
}

func NewLoopableIterator(tasks []CancellableTask, opts ...LoopableIteratorOption) *LoopableIterator
```

## Creating an Iterator

To create an iterator, define a slice of `CancellableTask` functions and use the `Iterator` function:

```go
tasks := []gronos.CancellableTask{
    func(ctx context.Context) error {
        fmt.Println("Task 1 executed")
        return nil
    },
    func(ctx context.Context) error {
        fmt.Println("Task 2 executed")
        return nil
    },
}

iterApp := gronos.Iterator(context.Background(), tasks)
```

## Adding an Iterator to Gronos

Once you've created an iterator, you can add it to your Gronos instance like any other application:

```go
g.Add("myIterator", iterApp)
```

## Examples

### Basic Iterator

```go
package main

import (
    "context"
    "fmt"
    "time"

    "github.com/davidroman0O/gronos"
)

func main() {
    ctx, cancel := context.WithCancel(context.Background())
    defer cancel()

    g, errChan := gronos.New[string](ctx, nil)

    tasks := []gronos.CancellableTask{
        func(ctx context.Context) error {
            fmt.Println("Task 1: Fetching data")
            time.Sleep(time.Second)
            return nil
        },
        func(ctx context.Context) error {
            fmt.Println("Task 2: Processing data")
            time.Sleep(2 * time.Second)
            return nil
        },
        func(ctx context.Context) error {
            fmt.Println("Task 3: Saving results")
            time.Sleep(time.Second)
            return nil
        },
    }

    iterApp := gronos.Iterator(context.Background(), tasks)
    g.Add("dataProcessingLoop", iterApp)

    // Run for 30 seconds
    time.Sleep(30 * time.Second)

    g.Shutdown()
    g.Wait()

    // Handle errors
    for err := range errChan {
        fmt.Printf("Error: %v\n", err)
    }
}
```

### Iterator with Error Handling and Hooks

```go
iterApp := gronos.Iterator(context.Background(), tasks, 
    gronos.WithLoopableIteratorOptions(
        gronos.WithOnError(func(err error) error {
            fmt.Printf("Error occurred: %v\n", err)
            if errors.Is(err, SomeCriticalError) {
                return gronos.ErrLoopCritical
            }
            return err
        }),
        gronos.WithBeforeLoop(func() error {
            fmt.Println("Starting new iteration")
            return nil
        }),
        gronos.WithAfterLoop(func() error {
            fmt.Println("Iteration completed")
            return nil
        }),
    ),
)

g.Add("advancedIterator", iterApp)
```

This example demonstrates how to create an iterator with custom error handling and hooks that run before and after each iteration loop.