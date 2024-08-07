# Gronos

[![Go Reference](https://pkg.go.dev/badge/github.com/davidroman0O/gronos.svg)](https://pkg.go.dev/github.com/davidroman0O/gronos)
[![Go Report Card](https://goreportcard.com/badge/github.com/davidroman0O/gronos)](https://goreportcard.com/report/github.com/davidroman0O/gronos)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)

Gronos is a concurrent application management library for Go, designed to simplify the process of managing multiple concurrent applications within a single program. It provides a structured approach to application lifecycle management, error handling, and inter-application communication.

## Table of Contents

- [Features](#features)
- [Installation](#installation)
- [Usage](#usage)
  - [Creating a Gronos Instance](#creating-a-gronos-instance)
  - [Defining Applications](#defining-applications)
  - [Adding Applications](#adding-applications)
  - [Starting and Stopping](#starting-and-stopping)
  - [Error Handling](#error-handling)
- [Advanced Usage](#advanced-usage)
  - [Worker](#worker)
  - [Iterator](#iterator)
  - [Internal Messaging](#internal-messaging)
  - [Watermill Integration](#watermill-integration)
- [Configuration](#configuration)
- [Best Practices](#best-practices)
- [Examples](#examples)
- [Detailed Documentation](#detailed-documentation)
- [Contributing](#contributing)
- [License](#license)

## Features

- **Concurrent Application Management**: Manage multiple applications running concurrently with ease.
- **Type-Safe Keys**: Use any comparable type as keys for your applications.
- **Dynamic Application Management**: Add or remove applications at runtime.
- **Graceful Shutdown**: Properly shut down all managed applications.
- **Error Propagation**: Centralized error handling for all managed applications.
- **Worker Functionality**: Easily create and manage periodic tasks.
- **Iterator Pattern**: Implement repeating sequences of tasks effortlessly.
- **Internal Messaging System**: Allow inter-application communication.
- **Flexible Configuration**: Customize behavior with various options.
- **Watermill Integration**: Incorporate event-driven architecture and message routing.

## Installation

To install Gronos, use `go get`:

```bash
go get github.com/davidroman0O/gronos
```

Ensure your `go.mod` file contains the following line:

```
require github.com/davidroman0O/gronos v<latest-version>
```

Replace `<latest-version>` with the most recent version of Gronos.

## Usage

### Creating a Gronos Instance

To create a new Gronos instance, use the `New` function with a basic "Hello World" application:

```go
import (
    "context"
    "fmt"
    "time"
    "github.com/davidroman0O/gronos"
)

ctx := context.Background()
g, errChan := gronos.New[string](ctx, map[string]gronos.RuntimeApplication{
    "hello-world": func(ctx context.Context, shutdown <-chan struct{}) error {
        ticker := time.NewTicker(time.Second)
        defer ticker.Stop()
        for {
            select {
            case <-ticker.C:
                fmt.Println("Hello, World!")
            case <-ctx.Done():
                return ctx.Err()
            case <-shutdown:
                return nil
            }
        }
    },
})
```

The `New` function returns a Gronos instance and an error channel. The generic parameter (in this case, `string`) defines the type of keys used to identify applications.

### Defining Applications

Applications in Gronos are defined as functions with the following signature:

```go
type RuntimeApplication func(ctx context.Context, shutdown <-chan struct{}) error
```

Here's an example of a simple application:

```go
func simpleApp(ctx context.Context, shutdown <-chan struct{}) error {
    for {
        select {
        case <-ctx.Done():
            return ctx.Err()
        case <-shutdown:
            return nil
        default:
            // Do work here
            time.Sleep(time.Second)
            fmt.Println("Working...")
        }
    }
}
```

### Adding Applications

Add applications to Gronos using the `Add` method:

```go
g.Add("myApp", simpleApp)
```

### Starting and Stopping

Gronos starts managing applications as soon as they're added. To stop all applications and shut down Gronos:

```go
g.Shutdown()
g.Wait()
```

### Error Handling

Errors from applications are sent to the error channel returned by `New`:

```go
go func() {
    for err := range errChan {
        log.Printf("Error: %v\n", err)
    }
}()
```

## Advanced Usage

### Worker

The `Worker` function creates applications that perform periodic tasks:

```go
worker := gronos.Worker(time.Second, gronos.NonBlocking, func(ctx context.Context) error {
    fmt.Println("Periodic task executed")
    return nil
})

g.Add("periodicTask", worker)
```

### Iterator

The `Iterator` function creates applications that execute a sequence of tasks in a loop:

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
g.Add("taskSequence", iterApp)
```

### Internal Messaging

Gronos provides an internal messaging system for communication between applications. The available public messages for runtime applications are:

```go
func communicatingApp(ctx context.Context, shutdown <-chan struct{}) error {
    bus, err := gronos.UseBus(ctx)
    if err != nil {
        return err
    }
    
    // Available messages:
    // Add a new runtime application
    done, addMsg := gronos.MsgAddRuntimeApplication("newApp", newAppFunc)
    bus(addMsg)
    <-done

    // Force cancel shutdown for an application
    bus(gronos.MsgForceCancelShutdown("appName", errors.New("force cancel reason")))

    // Force terminate shutdown for an application
    bus(gronos.MsgForceTerminateShutdown("appName"))
    
    // ... rest of the application logic
    return nil
}
```

These messages allow you to dynamically add new applications, force cancel a shutdown, or force terminate a shutdown for specific applications.

### Watermill Integration

Gronos provides integration with the Watermill library, allowing you to easily incorporate event-driven architecture and message routing into your applications.

```go
import (
    "github.com/davidroman0O/gronos"
    watermillext "github.com/davidroman0O/gronos/watermill"
)

func main() {
    ctx := context.Background()
    watermillMiddleware := watermillext.NewWatermillMiddleware[string](watermill.NewStdLogger(true, true))

    g, errChan := gronos.New[string](ctx, map[string]gronos.RuntimeApplication{
        "setup": setupApp,
    },
        gronos.WithExtension[string](watermillMiddleware),
    )

    // ... rest of your Gronos setup
}

func setupApp(ctx context.Context, shutdown <-chan struct{}) error {
    com, err := gronos.UseBus(ctx)
    if err != nil {
        return err
    }

    pubSub := gochannel.NewGoChannel(gochannel.Config{}, watermill.NewStdLogger(false, false))

    com(watermillext.MsgAddPublisher("pubsub", pubSub))
    com(watermillext.MsgAddSubscriber("pubsub", pubSub))

    // ... rest of your setup
    return nil
}
```

This integration allows you to use Watermill's powerful messaging capabilities within your Gronos applications, enabling sophisticated pub/sub patterns and message routing.

## Configuration

Gronos supports various configuration options:

```go
g, errChan := gronos.New[string](ctx, nil,
    gronos.WithShutdownBehavior[string](gronos.ShutdownAutomatic),
    gronos.WithGracePeriod[string](5 * time.Second),
    gronos.WithMinRuntime[string](10 * time.Second),
)
```

Available options:
- `WithShutdownBehavior`: Define how Gronos should handle shutdowns.
- `WithGracePeriod`: Set the grace period for shutdowns.
- `WithMinRuntime`: Set the minimum runtime before allowing shutdown.

## Best Practices

1. **Error Handling**: Always handle errors from the error channel to prevent goroutine leaks.
2. **Context Usage**: Use the provided context for cancellation and timeout management.
3. **Graceful Shutdown**: Implement proper shutdown logic in your applications to ensure clean exits.
4. **Resource Management**: Properly manage resources (e.g., close file handles, database connections) in your applications.
5. **Avoid Blocking**: In `Worker` and `Iterator` tasks, avoid long-running operations that could block other tasks.

## Examples

### Basic Usage

```go
package main

import (
    "context"
    "fmt"
    "log"
    "time"

    "github.com/davidroman0O/gronos"
)

func main() {
    ctx, cancel := context.WithCancel(context.Background())
    defer cancel()

    g, errChan := gronos.New[string](ctx, map[string]gronos.RuntimeApplication{
        "hello-world": func(ctx context.Context, shutdown <-chan struct{}) error {
            ticker := time.NewTicker(time.Second)
            defer ticker.Stop()
            for {
                select {
                case <-ticker.C:
                    fmt.Println("Hello, World!")
                case <-ctx.Done():
                    return ctx.Err()
                case <-shutdown:
                    return nil
                }
            }
        },
    })

    // Error handling goroutine
    go func() {
        for err := range errChan {
            log.Printf("Error: %v\n", err)
        }
    }()

    // Add another application
    g.Add("app1", func(ctx context.Context, shutdown <-chan struct{}) error {
        ticker := time.NewTicker(2 * time.Second)
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

    // Run for 10 seconds
    time.Sleep(10 * time.Second)

    // Shutdown Gronos
    g.Shutdown()

    // Wait for all applications to finish
    g.Wait()
}
```

### Using Worker and Iterator

```go
package main

import (
    "context"
    "fmt"
    "log"
    "time"

    "github.com/davidroman0O/gronos"
)

func main() {
    ctx, cancel := context.WithCancel(context.Background())
    defer cancel()

    g, errChan := gronos.New[string](ctx, nil)

    // Error handling
    go func() {
        for err := range errChan {
            log.Printf("Error: %v\n", err)
        }
    }()

    // Add a worker
    worker := gronos.Worker(time.Second, gronos.NonBlocking, func(ctx context.Context) error {
        fmt.Println("Worker task executed")
        return nil
    })
    g.Add("worker", worker)

    // Add an iterator
    tasks := []gronos.CancellableTask{
        func(ctx context.Context) error {
            fmt.Println("Iterator task 1")
            return nil
        },
        func(ctx context.Context) error {
            fmt.Println("Iterator task 2")
            return nil
        },
    }
    iterator := gronos.Iterator(context.Background(), tasks)
    g.Add("iterator", iterator)

    // Run for 10 seconds
    time.Sleep(10 * time.Second)

    // Shutdown and wait
    g.Shutdown()
    g.Wait()
}
```

## Detailed Documentation

For more detailed information about specific features, please refer to the following documents:

- [Core Concepts](./docs/core-concepts.md)
- [Worker Functionality](./docs/worker.md)
- [Iterator Functionality](./docs/iterator.md)
- [Internal Messaging](./docs/messaging.md)
- [Watermill Integration](./docs/watermill.md)

## Contributing

Contributions to Gronos are welcome! Please follow these steps:

1. Fork the repository
2. Create your feature branch (`git checkout -b feature/AmazingFeature`)
3. Commit your changes (`git commit -m 'Add some AmazingFeature'`)
4. Push to the branch (`git push origin feature/AmazingFeature`)
5. Open a Pull Request

Please make sure to update tests as appropriate and adhere to the existing coding style.

## License

Gronos is released under the MIT License. See the [LICENSE](LICENSE) file for details.

---

For more information, please check the [documentation](https://pkg.go.dev/github.com/davidroman0O/gronos) or open an issue on GitHub.