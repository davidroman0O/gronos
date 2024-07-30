# Gronos API Reference

This document provides a comprehensive reference for the Gronos API.

## Gronos Instance

### New

```go
func New[K comparable](ctx context.Context, init map[K]RuntimeApplication, opts ...Option[K]) *gronos[K]
```

Creates a new Gronos instance.

- `K`: A comparable type used as keys for applications (often `string`)
- `ctx`: Context for overall control
- `init`: Map of initial applications
- `opts`: Optional configuration options

Example:
```go
g := gronos.New[string](context.Background(), map[string]gronos.RuntimeApplication{
    "app1": myApp1,
    "app2": myApp2,
}, gronos.WithShutdownBehavior[string](gronos.ShutdownAutomatic))
```

### Start

```go
func (g *gronos[K]) Start() chan error
```

Starts the Gronos instance and returns an error channel.

Example:
```go
errChan := g.Start()
```

### Shutdown

```go
func (g *gronos[K]) Shutdown(opts ...ShutdownOption)
```

Initiates the shutdown process for all applications managed by the Gronos instance.

Example:
```go
g.Shutdown(gronos.WithTimeout(5 * time.Second))
```

### Wait

```go
func (g *gronos[K]) Wait()
```

Blocks until all applications managed by the Gronos instance have terminated.

Example:
```go
g.Wait()
```

### Add

```go
func (g *gronos[K]) Add(k K, v RuntimeApplication) error
```

Adds a new application to the Gronos instance with the given key and RuntimeApplication.

Example:
```go
err := g.Add("newApp", func(ctx context.Context, shutdown <-chan struct{}) error {
    // Application logic here
    return nil
})
```

## Worker

```go
func Worker(interval time.Duration, mode ExecutionMode, app TickingApplication) RuntimeApplication
```

Creates a RuntimeApplication that executes a TickingApplication at specified intervals.

Example:
```go
worker := gronos.Worker(time.Second, gronos.NonBlocking, func(ctx context.Context) error {
    fmt.Println("Periodic task executed")
    return nil
})
```

## Iterator

```go
func Iterator(iterCtx context.Context, tasks []CancellableTask, opts ...IteratorOption) RuntimeApplication
```

Creates a RuntimeApplication that uses a LoopableIterator to execute tasks.

Example:
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
```

## Message Creation Functions

Gronos provides several functions for creating different types of messages:

```go
func MsgDeadLetter[K comparable](k K, reason error) deadLetterMessage[K]
func MsgTerminated[K comparable](k K) terminatedMessage[K]
func MsgContextTerminated[K comparable](k K, err error) contextTerminatedMessage[K]
func MsgError[K comparable](k K, err error) errorMessage[K]
func MsgAdd[K comparable](k K, app RuntimeApplication) addMessage[K]
```

These functions help in creating properly structured messages for internal communication within Gronos.

## UseBus

```go
func UseBus(ctx context.Context) (chan<- Message, error)
```

Retrieves the communication channel from a context created by Gronos.

Example:
```go
bus, err := gronos.UseBus(ctx)
if err != nil {
    return err
}
bus <- gronos.MsgDeadLetter("appKey", errors.New("some error"))
```

For more detailed information about using these API functions and types, refer to the [Examples](examples.md) and [Best Practices](best-practices.md) sections.