# Gronos

Gronos is a minimalistic concurrent application management tool. 

It manages multiple concurrent applications, with features like dynamic application addition, customizable execution modes, and built-in error handling.

I was tired of writing the same boilerplates all the time so I wrote `gronos`, my now go-to for bootstrapping the runtime of my apps.

## Features

- Concurrent application management
- Dynamic application addition and removal
- Multiple execution modes (NonBlocking, ManagedTimeline, BestEffort)
- Built-in error handling and propagation
- Customizable clock system for timed executions
- Generic implementation allowing for type-safe keys

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

## Contributing

Contributions to Gronos are welcome! Please feel free to submit a Pull Request.

## License

Gronos is released under the MIT License. See the LICENSE file for details.