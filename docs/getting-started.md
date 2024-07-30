# Getting Started with Gronos

This guide will walk you through the process of setting up and using Gronos in your Go project.

## Installation

To start using Gronos, you need to install it in your Go project. Run the following command:

```bash
go get github.com/davidroman0O/gronos
```

## Basic Usage

Here's a more comprehensive example to get you started with Gronos:

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

    // Create a new Gronos instance with initial applications
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
            // Simulate some work
            time.Sleep(3 * time.Second)
            fmt.Println("App2 has completed its work")
            return nil
        },
    })

    // Start Gronos
    errChan := g.Start()

    // Dynamically add another application after starting
    err := g.Add("app3", func(ctx context.Context, shutdown <-chan struct{}) error {
        select {
        case <-time.After(4 * time.Second):
            fmt.Println("App3 has completed its work")
            return nil
        case <-ctx.Done():
            return ctx.Err()
        case <-shutdown:
            return nil
        }
    })

    if err != nil {
        log.Fatalf("Failed to add app3: %v", err)
    }

    // Run for 5 seconds
    time.Sleep(5 * time.Second)

    // Initiate shutdown
    g.Shutdown()

    // Wait for all applications to finish
    g.Wait()

    // Check for any errors
    for err := range errChan {
        if err != nil {
            fmt.Printf("Error: %v\n", err)
        }
    }
}
```

This example demonstrates the following key aspects of Gronos:

1. Creating a new Gronos instance with initial applications
2. Starting Gronos and obtaining an error channel
3. Dynamically adding an application after Gronos has started
4. Running applications concurrently
5. Initiating a shutdown and waiting for all applications to finish
6. Handling potential errors from all applications

## Key Concepts Demonstrated

- **Initialization with Applications**: Gronos is initialized with a map of `string` keys to `RuntimeApplication` functions. This allows you to set up multiple applications right from the start.
- **RuntimeApplication**: Each application is a function that takes a context and a shutdown channel. It should respect both for proper termination.
- **Dynamic Addition**: Applications can be added dynamically using the `Add` method, even after Gronos has started.
- **Concurrent Execution**: Gronos manages the concurrent execution of all added applications.
- **Graceful Shutdown**: The `Shutdown` method initiates a graceful shutdown of all applications.
- **Error Handling**: Errors from all applications are collected and can be processed after Gronos has finished.

## Next Steps

- To understand the core concepts behind Gronos in more detail, read the [Core Concepts](core-concepts.md) section.
- For more detailed information about Gronos functions and types, refer to the [API Reference](api-reference.md).
- To see more complex examples of Gronos in action, check out the [Examples](examples.md) page.
- For tips on using Gronos effectively, see the [Best Practices](best-practices.md) section.