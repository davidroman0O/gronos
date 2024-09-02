# Gronos Worker

The Worker functionality in Gronos allows you to create applications that perform periodic tasks. This document covers the usage and configuration of Workers in Gronos.

## Table of Contents

1. [Worker Function](#worker-function)
2. [ExecutionMode](#executionmode)
3. [Creating a Worker](#creating-a-worker)
4. [Adding a Worker to Gronos](#adding-a-worker-to-gronos)
5. [Examples](#examples)

## Worker Function

The `Worker` function creates a `RuntimeApplication` that executes a given task periodically:

```go
func Worker(interval time.Duration, mode ExecutionMode, app TickingRuntime, opts ...tickerOption) RuntimeApplication
```

- `interval`: The time duration between executions of the task.
- `mode`: The execution mode for the worker (see [ExecutionMode](#executionmode)).
- `app`: The `TickingRuntime` function to be executed periodically.
- `opts`: Optional configuration options for the worker.

## ExecutionMode

`ExecutionMode` defines how the worker should be executed:

```go
type ExecutionMode int

const (
    NonBlocking ExecutionMode = iota
    ManagedTimeline
    BestEffort
)
```

- `NonBlocking`: Tasks are executed asynchronously in their own goroutine.
- `ManagedTimeline`: Tasks are executed sequentially, maintaining the order of scheduled executions.
- `BestEffort`: Tasks are scheduled to run as close to their intended time as possible.

## Creating a Worker

To create a worker, you need to define a `TickingRuntime` function and use the `Worker` function:

```go
type TickingRuntime func(context.Context) error

worker := gronos.Worker(time.Second, gronos.NonBlocking, func(ctx context.Context) error {
    fmt.Println("Periodic task executed")
    return nil
})
```

## Adding a Worker to Gronos

Once you've created a worker, you can add it to your Gronos instance like any other application:

```go
g.Add("myWorker", worker)
```

## Examples

### Basic Worker

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

    worker := gronos.Worker(time.Second, gronos.NonBlocking, func(ctx context.Context) error {
        fmt.Println("Worker task executed")
        return nil
    })

    g.Add("myWorker", worker)

    // Run for 10 seconds
    time.Sleep(10 * time.Second)

    g.Shutdown()
    g.Wait()

    // Handle errors
    for err := range errChan {
        fmt.Printf("Error: %v\n", err)
    }
}
```

### Worker with Error Handling

```go
worker := gronos.Worker(time.Second, gronos.ManagedTimeline, func(ctx context.Context) error {
    // Simulate an error every 5 executions
    if time.Now().Unix()%5 == 0 {
        return errors.New("simulated error")
    }
    fmt.Println("Worker task executed successfully")
    return nil
})

g.Add("errorProneWorker", worker)
```

This example demonstrates how to create a worker that may occasionally return an error, which will be sent to the error channel for centralized handling.