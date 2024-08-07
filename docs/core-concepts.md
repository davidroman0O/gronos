# Gronos Core Concepts

Gronos is a concurrent application management library for Go. It provides a framework for managing multiple concurrent applications within a single program. This document covers the core concepts and basic usage of Gronos.

## Table of Contents

1. [RuntimeApplication](#runtimeapplication)
2. [Creating a Gronos Instance](#creating-a-gronos-instance)
3. [Adding Applications](#adding-applications)
4. [Shutdown and Wait](#shutdown-and-wait)
5. [Error Handling](#error-handling)

## RuntimeApplication

A `RuntimeApplication` is the basic unit of execution in Gronos. It's defined as a function with the following signature:

```go
type RuntimeApplication func(ctx context.Context, shutdown <-chan struct{}) error
```

- `ctx`: A context for cancellation and value passing.
- `shutdown`: A channel that signals when the application should shut down.
- The function should return an error if any occurs during execution.

Example:

```go
func myApp(ctx context.Context, shutdown <-chan struct{}) error {
    ticker := time.NewTicker(time.Second)
    defer ticker.Stop()

    for {
        select {
        case <-ticker.C:
            fmt.Println("Working...")
        case <-ctx.Done():
            return ctx.Err()
        case <-shutdown:
            return nil
        }
    }
}
```

## Creating a Gronos Instance

To create a new Gronos instance, use the `New` function:

```go
func New[K comparable](ctx context.Context, init map[K]RuntimeApplication, opts ...Option[K]) (*gronos[K], chan error)
```

- `K`: The type of keys used to identify applications (must be comparable).
- `ctx`: A context for the Gronos instance.
- `init`: A map of initial applications to start with.
- `opts`: Optional configuration options.

Example:

```go
ctx := context.Background()
g, errChan := gronos.New[string](ctx, map[string]gronos.RuntimeApplication{
    "app1": myApp,
})
```

## Adding Applications

You can add applications dynamically using the `Add` method:

```go
func (g *gronos[K]) Add(k K, v RuntimeApplication, opts ...addOption) <-chan struct{}
```

- `k`: The key to identify the application.
- `v`: The RuntimeApplication to add.
- `opts`: Optional configuration options for adding the application.

Example:

```go
done := g.Add("app2", myOtherApp)
<-done // Wait for the application to be added
```

## Shutdown and Wait

To initiate a shutdown of all applications:

```go
func (g *gronos[K]) Shutdown()
```

To wait for all applications to finish:

```go
func (g *gronos[K]) Wait()
```

Example:

```go
g.Shutdown()
g.Wait()
```

## Error Handling

Errors from applications are sent to the error channel returned by `New`:

```go
go func() {
    for err := range errChan {
        log.Printf("Error: %v\n", err)
    }
}()
```

This allows for centralized error handling for all managed applications.