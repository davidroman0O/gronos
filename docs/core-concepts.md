# Core Concepts in Gronos

Understanding the core concepts of Gronos is crucial for effectively using the library. This document outlines the key components and ideas behind Gronos.

## 1. RuntimeApplication

A `RuntimeApplication` is the basic unit of execution in Gronos. It's defined as a function with the following signature:

```go
type RuntimeApplication func(ctx context.Context, shutdown <-chan struct{}) error
```

This function represents a long-running process that can be managed by Gronos. It should respect both the context and the shutdown channel for proper termination.

Example:
```go
func myApp(ctx context.Context, shutdown <-chan struct{}) error {
    for {
        select {
        case <-ctx.Done():
            return ctx.Err()
        case <-shutdown:
            return nil
        default:
            // Do work here
        }
    }
}
```

## 2. Gronos Instance

The Gronos instance is the main controller for managing multiple RuntimeApplications. It's created using the `New` function:

```go
g := gronos.New[K](ctx, initialApps, opts...)
```

Where:
- `K` is a comparable type used as keys for applications (often `string`)
- `ctx` is a context for overall control
- `initialApps` is a map of initial applications
- `opts` are optional configuration options

Example:
```go
g := gronos.New[string](ctx, map[string]gronos.RuntimeApplication{
    "app1": myApp1,
    "app2": myApp2,
})
```

## 3. Application Lifecycle

Applications in Gronos go through several stages:

- **Added**: The application is registered with Gronos
- **Starting**: The application is beginning its execution
- **Running**: The application is actively executing
- **Completed**: The application has finished successfully
- **Failed**: The application has terminated with an error

You can check the status of an application using methods like `IsStarted` and `IsComplete`.

## 4. Execution Modes

Gronos supports different execution modes for workers and clock tickers:

- **NonBlocking**: Tasks are executed asynchronously in their own goroutine
- **ManagedTimeline**: Tasks are executed sequentially, maintaining order
- **BestEffort**: Tasks are scheduled to run as close to their intended time as possible

Example:
```go
worker := gronos.Worker(time.Second, gronos.NonBlocking, func(ctx context.Context) error {
    // Periodic task logic
    return nil
})
```

## 5. Message System

Gronos uses an internal message system for communication between components. Key message types include:

- **DeadLetter**: Indicates an application has terminated with an error
- **Terminated**: Indicates an application has terminated normally
- **ContextTerminated**: Indicates an application's context has been terminated

You can send messages within a RuntimeApplication using the `UseBus` function:

```go
bus, err := gronos.UseBus(ctx)
if err != nil {
    return err
}
bus <- gronos.MsgDeadLetter("appKey", errors.New("some error"))
```

## 6. Worker and Clock

Gronos provides a `Worker` function for creating applications that perform periodic tasks, and a `Clock` system for timed executions.

Example of a Worker:
```go
worker := gronos.Worker(time.Second, gronos.NonBlocking, func(ctx context.Context) error {
    fmt.Println("Periodic task executed")
    return nil
})
g.Add("periodicTask", worker)
```

## 7. Iterator and Loopable Iterator

These components allow for managing sequences of tasks and provide advanced control over task execution and error handling.

Example of an Iterator:
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

iterApp := gronos.Iterator(context.Background(), tasks)
g.Add("iteratorApp", iterApp)
```

## 8. Shutdown and Error Handling

Gronos provides mechanisms for graceful shutdown and comprehensive error handling:

```go
// Initiate shutdown
g.Shutdown()

// Wait for all applications to finish
g.Wait()

// Handle errors
for err := range errChan {
    if err != nil {
        fmt.Printf("Error: %v\n", err)
    }
}
```

Understanding these core concepts provides a solid foundation for working with Gronos. In the following sections, we'll explore how to use these concepts in practice and dive deeper into advanced usage patterns.

## Next Steps

- For detailed information about Gronos functions and types, refer to the [API Reference](api-reference.md).
- To see more complex examples of Gronos in action, check out the [Examples](examples.md) page.
- For tips on using Gronos effectively, see the [Best Practices](best-practices.md) section.