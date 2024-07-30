# Gronos Examples

This document provides various examples of how to use Gronos in different scenarios. These examples demonstrate key features and common use cases.

## Table of Contents

1. [Basic Usage](#basic-usage)
2. [Dynamic Application Addition](#dynamic-application-addition)
3. [Using Workers](#using-workers)
4. [Using Iterators](#using-iterators)
5. [Custom Shutdown Behavior](#custom-shutdown-behavior)
6. [Error Handling](#error-handling)
7. [Inter-Application Communication](#inter-application-communication)

## Basic Usage

This example demonstrates the basic usage of Gronos with multiple applications.

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

    g := gronos.New[string](ctx, map[string]gronos.RuntimeApplication{
        "app1": func(ctx context.Context, shutdown <-chan struct{}) error {
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
        },
        "app2": func(ctx context.Context, shutdown <-chan struct{}) error {
            time.Sleep(3 * time.Second)
            fmt.Println("App2 has completed its work")
            return nil
        },
    })

    errChan := g.Start()

    time.Sleep(5 * time.Second)
    g.Shutdown()
    g.Wait()

    for err := range errChan {
        if err != nil {
            fmt.Printf("Error: %v\n", err)
        }
    }
}
```

## Dynamic Application Addition

This example shows how to dynamically add applications to a running Gronos instance.

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

    g := gronos.New[string](ctx, nil)

    errChan := g.Start()

    // Add applications dynamically
    g.Add("app1", func(ctx context.Context, shutdown <-chan struct{}) error {
        fmt.Println("App1 started")
        <-shutdown
        fmt.Println("App1 shutting down")
        return nil
    })

    time.Sleep(2 * time.Second)

    g.Add("app2", func(ctx context.Context, shutdown <-chan struct{}) error {
        fmt.Println("App2 started")
        <-shutdown
        fmt.Println("App2 shutting down")
        return nil
    })

    time.Sleep(3 * time.Second)
    g.Shutdown()
    g.Wait()

    for err := range errChan {
        if err != nil {
            fmt.Printf("Error: %v\n", err)
        }
    }
}
```

## Using Workers

This example demonstrates how to use Gronos workers for periodic tasks.

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

    g := gronos.New[string](ctx, nil)

    worker := gronos.Worker(time.Second, gronos.NonBlocking, func(ctx context.Context) error {
        fmt.Println("Worker task executed")
        return nil
    })

    g.Add("worker", worker)

    errChan := g.Start()

    time.Sleep(5 * time.Second)
    g.Shutdown()
    g.Wait()

    for err := range errChan {
        if err != nil {
            fmt.Printf("Error: %v\n", err)
        }
    }
}
```

## Using Iterators

This example shows how to use Gronos iterators for managing sequences of tasks.

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

    tasks := []gronos.CancellableTask{
        func(ctx context.Context) error {
            fmt.Println("Task 1 executed")
            time.Sleep(time.Second)
            return nil
        },
        func(ctx context.Context) error {
            fmt.Println("Task 2 executed")
            time.Sleep(time.Second)
            return nil
        },
    }

    iterApp := gronos.Iterator(ctx, tasks)

    g := gronos.New[string](ctx, map[string]gronos.RuntimeApplication{
        "iterator": iterApp,
    })

    errChan := g.Start()

    time.Sleep(10 * time.Second)
    g.Shutdown()
    g.Wait()

    for err := range errChan {
        if err != nil {
            fmt.Printf("Error: %v\n", err)
        }
    }
}
```

## Custom Shutdown Behavior

This example demonstrates how to use custom shutdown behavior in Gronos.

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

    g := gronos.New[string](ctx, map[string]gronos.RuntimeApplication{
        "app1": func(ctx context.Context, shutdown <-chan struct{}) error {
            <-shutdown
            fmt.Println("App1 received shutdown signal")
            time.Sleep(2 * time.Second) // Simulate cleanup
            fmt.Println("App1 finished cleanup")
            return nil
        },
    }, gronos.WithShutdownBehavior[string](gronos.ShutdownManual))

    errChan := g.Start()

    time.Sleep(3 * time.Second)
    fmt.Println("Initiating shutdown")
    g.Shutdown(gronos.WithTimeout(5 * time.Second))
    g.Wait()

    for err := range errChan {
        if err != nil {
            fmt.Printf("Error: %v\n", err)
        }
    }
}
```

## Error Handling

This example shows how to handle errors in Gronos applications.

```go
package main

import (
    "context"
    "errors"
    "fmt"
    "time"

    "github.com/davidroman0O/gronos"
)

func main() {
    ctx, cancel := context.WithCancel(context.Background())
    defer cancel()

    g := gronos.New[string](ctx, map[string]gronos.RuntimeApplication{
        "app1": func(ctx context.Context, shutdown <-chan struct{}) error {
            return errors.New("app1 error")
        },
        "app2": func(ctx context.Context, shutdown <-chan struct{}) error {
            time.Sleep(2 * time.Second)
            return nil
        },
    })

    errChan := g.Start()

    g.Wait()

    for err := range errChan {
        if err != nil {
            fmt.Printf("Error: %v\n", err)
        }
    }
}
```

## Inter-Application Communication

This example demonstrates how to use the message bus for communication between applications.

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

    g := gronos.New[string](ctx, map[string]gronos.RuntimeApplication{
        "sender": func(ctx context.Context, shutdown <-chan struct{}) error {
            bus, err := gronos.UseBus(ctx)
            if err != nil {
                return err
            }
            time.Sleep(2 * time.Second)
            bus <- gronos.MsgAdd("dynamicApp", func(ctx context.Context, shutdown <-chan struct{}) error {
                fmt.Println("Dynamic app started")
                <-shutdown
                return nil
            })
            return nil
        },
    })

    errChan := g.Start()

    time.Sleep(5 * time.Second)
    g.Shutdown()
    g.Wait()

    for err := range errChan {
        if err != nil {
            fmt.Printf("Error: %v\n", err)
        }
    }
}
```

These examples cover a range of Gronos features and use cases. For more information on best practices and advanced usage, refer to the [Best Practices](best-practices.md) and [Advanced Usage](advanced-usage.md) sections.