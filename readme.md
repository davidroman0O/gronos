# Gronos

Gronos is a minimalistic concurrent application management tool. 

It manages multiple concurrent applications, with features like dynamic application addition, customizable execution modes, and built-in error handling.

I was tired of writing the same boilerplates all the time so I wrote `gronos`, my now go-to for bootstrapping the runtime of my apps.

<img src="gronos.webp" alt="Gronos" height="400">

## Table of Contents

1. [Features](#features)
2. [Installation](#installation)
3. [Quick Start](#quick-start)
4. [Usage](#usage)
   - [Creating a Gronos Instance](#creating-a-gronos-instance)
   - [Adding Applications](#adding-applications)
   - [Starting Gronos](#starting-gronos)
   - [Shutting Down](#shutting-down)
   - [Waiting for Completion](#waiting-for-completion)
   - [Using the Worker](#using-the-worker)
   - [Using the Clock](#using-the-clock)
   - [ExecutionMode](#executionmode)
   - [Using the Iterator](#using-the-iterator)
   - [Loopable Iterator](#loopable-iterator)
   - [Message Creation Functions](#message-creation-functions)
5. [Contributing](#contributing)
6. [License](#license)

## Features

- Concurrent application management
- Dynamic application addition and removal
- Multiple execution modes (NonBlocking, ManagedTimeline, BestEffort)
- Built-in error handling and propagation
- Customizable clock system for timed executions
- Generic implementation allowing for type-safe keys
- Iterator for managing sequences of tasks
- Loopable Iterator for advanced task management
- Comprehensive message handling system

## Installation

To install Gronos, use `go get`:

```bash
go get github.com/davidroman0O/gronos
```

## Quick Start

Here's a simple example to get you started with Gronos:

```go
package main

import (
    "context"
    "fmt"
    "time"

    "github.com/davidroman0O/gronos"
)

func main() {
    ctx := context.Background()
    
    // Create a new Gronos instance
    g := gronos.New[string](ctx, nil)

    // Add a simple application
    g.Add("app1", func(ctx context.Context, shutdown <-chan struct{}) error {
        ticker := time.NewTicker(time.Second)
        defer ticker.Stop()

        for {
            select {
            case <-ticker.C:
                fmt.Println("App1 is running")
            case <-ctx.Done():
                return ctx.Err()
            case <-shutdown:
                return nil
            }
        }
    })

    // Start Gronos
    errChan := g.Start()

    // Run for 5 seconds
    time.Sleep(5 * time.Second)

    // Shutdown Gronos
    g.Shutdown()

    // Wait for all applications to finish
    g.Wait()

    // Check for any errors
    if err := <-errChan; err != nil {
        fmt.Printf("Error: %v\n", err)
    }
}
```

## Usage

### Creating a Gronos Instance

Create a new Gronos instance using the `New` function:

```go
g := gronos.New[string](ctx, nil)
```

The generic parameter `string` specifies the type of keys used to identify applications. You can use any comparable type as the key.

### Adding Applications

Add applications to Gronos using the `Add` method:

```go
g.Add("myApp", func(ctx context.Context, shutdown <-chan struct{}) error {
    // Application logic here
    return nil
})
```

### Starting Gronos

Start the Gronos instance and get an error channel:

```go
errChan := g.Start()
```

### Shutting Down

To shut down all applications managed by Gronos:

```go
g.Shutdown()
```

You can also specify a timeout for the shutdown process:

```go
g.Shutdown(gronos.WithTimeout(5 * time.Second))
```

### Waiting for Completion

Wait for all applications to finish:

```go
g.Wait()
```

### Using the Worker

Gronos provides a `Worker` function to create applications that perform periodic tasks:

```go
worker := gronos.Worker(time.Second, gronos.NonBlocking, func(ctx context.Context) error {
    fmt.Println("Periodic task executed")
    return nil
})

g.Add("periodicTask", worker)
```

### Using the Clock

Gronos includes a flexible `Clock` system for timed executions:

```go
clock := gronos.NewClock(
    gronos.WithName("MyClock"),
    gronos.WithInterval(time.Second),
)

clock.Add(&MyTicker{}, gronos.NonBlocking)
clock.Start()
```

### ExecutionMode

Gronos supports different execution modes for workers and clock tickers. These modes define how tasks are scheduled and executed:

1. **NonBlocking**
   - Tasks are executed asynchronously in their own goroutine.
   - Suitable for tasks that can run independently and don't need to maintain strict timing.
   - Example use case: Background data processing that doesn't need precise scheduling.

2. **ManagedTimeline**
   - Tasks are executed sequentially, maintaining the order of scheduled executions.
   - If a task takes longer than the scheduled interval, subsequent executions are delayed to maintain the timeline.
   - Suitable for tasks where the order of execution is important and missed executions should be caught up.
   - Example use case: Financial transactions that must be processed in order.

3. **BestEffort**
   - Tasks are scheduled to run as close to their intended time as possible.
   - If a task is delayed (e.g., due to system load), the system will try to catch up, but may skip executions if too far behind.
   - Strikes a balance between maintaining schedule and system performance.
   - Example use case: Regular system health checks where occasional missed checks are acceptable.

When using `Worker` or `Clock.Add()`, you can specify the execution mode:

```go
worker := gronos.Worker(time.Second, gronos.ManagedTimeline, func(ctx context.Context) error {
    // This task will maintain its timeline, even if executions are delayed
    return nil
})

clock.Add(&MyTicker{}, gronos.BestEffort)
```

Choose the appropriate execution mode based on your task's requirements for timing accuracy, order preservation, and system load considerations.

### Using the Iterator

Gronos provides an `Iterator` function to create applications that execute a sequence of tasks in a continuous loop:

```go
func Iterator(iterCtx context.Context, tasks []CancellableTask, opts ...IteratorOption) RuntimeApplication
```

The `Iterator` creates a `RuntimeApplication` that uses a `LoopableIterator` to execute a series of tasks repeatedly. It continues looping through the tasks until it's explicitly stopped or encounters a critical error.

Key characteristics of the Iterator:

1. It executes a predefined sequence of tasks in order.
2. After completing all tasks, it starts over from the beginning.
3. It continues this loop until stopped or a critical error occurs.
4. It provides fine-grained control over error handling and iteration behavior.

Example usage:

```go
tasks := []gronos.CancellableTask{
    func(ctx context.Context) error {
        fmt.Println("Task 1: Fetching data")
        // Simulating work
        time.Sleep(time.Second)
        return nil
    },
    func(ctx context.Context) error {
        fmt.Println("Task 2: Processing data")
        // Simulating work
        time.Sleep(2 * time.Second)
        return nil
    },
    func(ctx context.Context) error {
        fmt.Println("Task 3: Saving results")
        // Simulating work
        time.Sleep(time.Second)
        return nil
    },
}

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

g.Add("dataProcessingLoop", iterApp)
```

In this example, the Iterator will continuously run these three tasks in sequence. After completing Task 3, it will start over with Task 1. This loop continues until the application is shut down or a critical error occurs.

Advanced usage with dynamic intervals:

```go
var lastInterval atomic.Int64
lastInterval.Store(1000) // Start with 1 second interval

tasks := []gronos.CancellableTask{
    func(ctx context.Context) error {
        interval := lastInterval.Load()
        fmt.Printf("Current interval: %dms\n", interval)
        time.Sleep(time.Duration(interval) * time.Millisecond)
        
        // Increase interval by 500ms each iteration, up to 5 seconds
        newInterval := interval + 500
        if newInterval > 5000 {
            newInterval = 1000 // Reset to 1 second
        }
        lastInterval.Store(newInterval)
        return nil
    },
}

iterApp := gronos.Iterator(context.Background(), tasks)

g.Add("dynamicIntervalApp", iterApp)
```

This example demonstrates how you can create an Iterator that dynamically adjusts its own timing between iterations.

The `Iterator` function accepts the following parameters:

- `iterCtx`: A context for controlling the iterator's lifecycle.
- `tasks`: A slice of `CancellableTask` functions to be executed in sequence.
- `opts`: Optional `IteratorOption` for configuring the iterator's behavior.

Available `IteratorOption`:

- `WithLoopableIteratorOptions`: Allows you to pass options to the underlying `LoopableIterator`, including:
  - `WithOnError`: Custom error handling function.
  - `WithBeforeLoop`: Function to execute before each iteration.
  - `WithAfterLoop`: Function to execute after each iteration.
  - `WithExtraCancel`: Additional cancel functions to be called when the iterator is cancelled.

The Iterator provides several advantages:

1. It allows you to define a series of tasks that will be executed in sequence repeatedly.
2. It provides granular control over error handling and iteration behavior.
3. It integrates seamlessly with the Gronos runtime, allowing you to manage complex, repeating task sequences as a single application.
4. It's highly customizable through the use of various options and hooks.

### Loopable Iterator

Gronos includes a `LoopableIterator` for more advanced task management:

```go
tasks := []gronos.CancellableTask{
    func(ctx context.Context) error {
        // Task 1 logic
        return nil
    },
    func(ctx context.Context) error {
        // Task 2 logic
        return nil
    },
}

iterator := gronos.NewLoopableIterator(tasks,
    gronos.WithBeforeLoop(func() error {
        // Logic to run before each iteration
        return nil
    }),
    gronos.WithAfterLoop(func() error {
        // Logic to run after each iteration
        return nil
    }),
)

errChan := iterator.Run(ctx)
```


### Message Creation Functions

Gronos provides functions for creating different types of messages for internal communication:

```go
deadLetter := gronos.MsgDeadLetter("appKey", errors.New("application error"))
terminated := gronos.MsgTerminated("appKey")
ctxTerminated := gronos.MsgContextTerminated("appKey", context.Canceled)
errMsg := gronos.MsgError("appKey", errors.New("custom error"))
addMsg := gronos.MsgAdd("newAppKey", myNewApp)
```

These functions help in creating properly structured messages for internal communication within Gronos.

### Communication within a RuntimeApplication

Use the `ctx` with `gronos.UseBus` to send messages 

```go
func(ctx context.Context, shutdown <-chan struct{}) error {
        bus, err := gronos.UseBus(ctx)
        if err != nil {
            return err
        }
        bus <- gronos.MsgContextTerminated("iteratorApp", nil) // will execute the extra cancel
        bus <- gronos.MsgDeadLetter("asideWorker", nil)        // will just stop shutdown
        return nil
}
```


## Contributing

Contributions to Gronos are welcome! Please feel free to submit a Pull Request.

## License

Gronos is released under the MIT License. See the LICENSE file for details.