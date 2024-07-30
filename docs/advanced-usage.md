# Advanced Usage of Gronos

This document covers advanced usage patterns and techniques for Gronos, suitable for users who are already familiar with its basic concepts.

## Table of Contents

1. [Custom Extension Hooks](#custom-extension-hooks)
2. [Dynamic Interval Workers](#dynamic-interval-workers)
3. [Complex Error Handling with Loopable Iterator](#complex-error-handling-with-loopable-iterator)
<!-- 4. [Implementing a Supervisor Pattern](#implementing-a-supervisor-pattern) -->
<!-- 5. [Using Gronos with External Event Sources](#using-gronos-with-external-event-sources) -->
<!-- 6. [Implementing Graceful Degradation](#implementing-graceful-degradation) -->
<!-- 7. [Advanced Shutdown Strategies](#advanced-shutdown-strategies) -->

## Custom Extension Hooks

Gronos allows you to create custom extensions to hook into its lifecycle events. Here's an example of a custom extension:

```go
type MyExtension[K comparable] struct{}

func (e *MyExtension[K]) OnStart(ctx context.Context, errChan chan<- error) error {
    fmt.Println("MyExtension: Starting")
    return nil
}

func (e *MyExtension[K]) OnStop(ctx context.Context, errChan chan<- error) error {
    fmt.Println("MyExtension: Stopping")
    return nil
}

func (e *MyExtension[K]) OnNewRuntime(ctx context.Context) context.Context {
    return context.WithValue(ctx, "myExtensionKey", "myExtensionValue")
}

func (e *MyExtension[K]) OnMsg(ctx context.Context, m gronos.Message) error {
    fmt.Printf("MyExtension: Received message %T\n", m)
    return nil
}

// Usage
g := gronos.New[string](ctx, initialApps, gronos.WithExtension[string](&MyExtension[string]{}))
```

## Dynamic Interval Workers

You can create workers with dynamic intervals that change based on certain conditions:

```go
var interval atomic.Int64
interval.Store(1000) // Start with 1 second interval

worker := gronos.Worker(time.Millisecond, gronos.NonBlocking, func(ctx context.Context) error {
    currentInterval := time.Duration(interval.Load()) * time.Millisecond
    fmt.Printf("Working with interval: %v\n", currentInterval)
    
    // Simulate work
    time.Sleep(currentInterval)
    
    // Change interval based on some condition
    if someCondition() {
        newInterval := currentInterval * 2
        interval.Store(int64(newInterval / time.Millisecond))
    }
    
    return nil
})

g.Add("dynamicWorker", worker)
```


## Complex Error Handling with Loopable Iterator

Use the Loopable Iterator for advanced error handling and flow control:

```go
tasks := []gronos.CancellableTask{
    func(ctx context.Context) error {
        // Task 1 logic
        return nil
    },
    func(ctx context.Context) error {
        // Task 2 logic
        return errors.New("task 2 error")
    },
}

iterApp := gronos.Iterator(ctx, tasks,
    gronos.WithLoopableIteratorOptions(
        gronos.WithOnError(func(err error) error {
            if errors.Is(err, SomeSpecificError) {
                return gronos.ErrLoopCritical
            }
            log.Printf("Non-critical error: %v", err)
            return nil
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

g.Add("complexIterator", iterApp)
```

<!-- 
## Implementing a Supervisor Pattern

You can implement a supervisor pattern to monitor and restart failed applications:

```go
func supervisorApp(ctx context.Context, shutdown <-chan struct{}) error {
    bus, err := gronos.UseBus(ctx)
    if err != nil {
        return err
    }

    for {
        select {
        case <-ctx.Done():
            return ctx.Err()
        case <-shutdown:
            return nil
        case msg := <-bus:
            if deadLetter, ok := msg.(gronos.deadLetterMessage[string]); ok {
                fmt.Printf("Application %s failed: %v\n", deadLetter.Key, deadLetter.Reason)
                
                // Restart the failed application
                bus <- gronos.MsgAdd(deadLetter.Key, createApp(deadLetter.Key))
            }
        }
    }
}

g.Add("supervisor", supervisorApp)

func createApp(key string) gronos.RuntimeApplication {
    return func(ctx context.Context, shutdown <-chan struct{}) error {
        // Application logic
        return nil
    }
}
``` -->

## Using Gronos with External Event Sources

Integrate Gronos with external event sources like message queues:

```go
func mqConsumer(ctx context.Context, shutdown <-chan struct{}) error {
    consumer := createMQConsumer() // Implement this function to create your message queue consumer
    
    for {
        select {
        case <-ctx.Done():
            return ctx.Err()
        case <-shutdown:
            return nil
        default:
            msg, err := consumer.Receive(ctx)
            if err != nil {
                log.Printf("Error receiving message: %v", err)
                continue
            }
            
            // Process the message
            processMessage(msg)
        }
    }
}

g.Add("mqConsumer", mqConsumer)
```

<!-- 
## Implementing Graceful Degradation

Implement graceful degradation to handle resource constraints:

```go
func appWithGracefulDegradation(ctx context.Context, shutdown <-chan struct{}) error {
    normalMode := true
    checkResources := time.NewTicker(time.Minute)
    defer checkResources.Stop()

    for {
        select {
        case <-ctx.Done():
            return ctx.Err()
        case <-shutdown:
            return nil
        case <-checkResources.C:
            if resourcesConstrained() {
                if normalMode {
                    log.Println("Switching to degraded mode")
                    normalMode = false
                }
                performDegradedOperation()
            } else {
                if !normalMode {
                    log.Println("Switching to normal mode")
                    normalMode = true
                }
                performNormalOperation()
            }
        }
    }
}

g.Add("degradableApp", appWithGracefulDegradation)
``` -->

<!-- 
## Advanced Shutdown Strategies

Implement advanced shutdown strategies to ensure all applications shut down in the correct order:

```go
func orderedShutdown(g *gronos.Gronos[string]) {
    shutdownOrder := []string{"app3", "app2", "app1"}
    
    for _, appKey := range shutdownOrder {
        fmt.Printf("Shutting down %s\n", appKey)
        g.com <- gronos.MsgDeadLetter(appKey, errors.New("shutdown"))
        for g.IsRunning(appKey) {
            time.Sleep(100 * time.Millisecond)
        }
        fmt.Printf("%s shut down complete\n", appKey)
    }
}

// Use this function instead of g.Shutdown() for ordered shutdown
``` -->

These advanced usage patterns demonstrate the flexibility and power of Gronos for complex application management scenarios. For more information on best practices and common issues, refer to the [Best Practices](best-practices.md) and [Troubleshooting](troubleshooting.md) sections.