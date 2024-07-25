# Gronos

Gronos is a minimalistic concurrent application management tool. 

It manage multiple concurrent applications, with features like dynamic application addition, customizable execution modes, and built-in error handling.

I was tired of writting the same boilerplates all the time so I wrote `gronos`, my now go-to for bootstrapping the runtime of my apps.

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

## Contributing

Contributions to Gronos are welcome! Please feel free to submit a Pull Request.

## License

Gronos is released under the MIT License. See the LICENSE file for details.
