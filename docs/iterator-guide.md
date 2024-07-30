# Comprehensive Guide to Gronos Iterator

The Gronos Iterator is a powerful tool for managing sequences of tasks with fine-grained control over execution flow, error handling, and lifecycle hooks. This guide provides an in-depth look at the Iterator, its options, and various use cases.

## Table of Contents

1. [Introduction to the Iterator](#introduction-to-the-iterator)
2. [Basic Usage](#basic-usage)
3. [Iterator Options](#iterator-options)
   - [WithOnError](#withonerror)
   - [WithShouldStop](#withshouldstop)
   - [WithBeforeLoop](#withbeforeloop)
   - [WithAfterLoop](#withafterloop)
   - [WithExtraCancel](#withextracancel)
4. [Advanced Use Cases](#advanced-use-cases)
   - [Error Handling and Recovery](#error-handling-and-recovery)
   - [Conditional Loop Termination](#conditional-loop-termination)
   - [State Management Between Iterations](#state-management-between-iterations)
   - [Dynamic Task Modification](#dynamic-task-modification)
   - [Timed Iterations](#timed-iterations)
5. [Best Practices](#best-practices)
6. [Common Pitfalls](#common-pitfalls)

## Introduction to the Iterator

The Gronos Iterator allows you to define a sequence of tasks (called `CancellableTask`s) that are executed in order, repeatedly, until a termination condition is met. It provides hooks for error handling, pre- and post-iteration actions, and fine-grained control over the iteration process.

## Basic Usage

Here's a simple example of using the Gronos Iterator:

```go
tasks := []gronos.CancellableTask{
    func(ctx context.Context) error {
        fmt.Println("Task 1")
        return nil
    },
    func(ctx context.Context) error {
        fmt.Println("Task 2")
        return nil
    },
}

iterApp := gronos.Iterator(context.Background(), tasks)

g := gronos.New[string](context.Background(), map[string]gronos.RuntimeApplication{
    "iterator": iterApp,
})

errChan := g.Start()
// ... handle errors and shutdown
```

This will create an iterator that continuously executes Task 1 followed by Task 2 until the Gronos instance is shut down.

## Iterator Options

The Iterator can be customized with several options to control its behavior:

### WithOnError

This option allows you to define custom error handling logic:

```go
iterApp := gronos.Iterator(ctx, tasks,
    gronos.WithLoopableIteratorOptions(
        gronos.WithOnError(func(err error) error {
            log.Printf("Error occurred: %v", err)
            if errors.Is(err, SomeCriticalError) {
                return gronos.ErrLoopCritical
            }
            return nil // continue iteration
        }),
    ),
)
```

### WithShouldStop

This option lets you define a condition for stopping the iteration:

```go
var iterationCount int
iterApp := gronos.Iterator(ctx, tasks,
    gronos.WithLoopableIteratorOptions(
        gronos.WithShouldStop(func(err error) bool {
            iterationCount++
            return iterationCount >= 10 || err != nil
        }),
    ),
)
```

### WithBeforeLoop

This option allows you to perform actions before each iteration:

```go
iterApp := gronos.Iterator(ctx, tasks,
    gronos.WithLoopableIteratorOptions(
        gronos.WithBeforeLoop(func() error {
            fmt.Println("Starting new iteration")
            return nil
        }),
    ),
)
```

### WithAfterLoop

This option allows you to perform actions after each iteration:

```go
iterApp := gronos.Iterator(ctx, tasks,
    gronos.WithLoopableIteratorOptions(
        gronos.WithAfterLoop(func() error {
            fmt.Println("Iteration completed")
            return nil
        }),
    ),
)
```

### WithExtraCancel

This option allows you to add extra cancellation functions:

```go
cleanup := func() {
    fmt.Println("Performing extra cleanup")
}

iterApp := gronos.Iterator(ctx, tasks,
    gronos.WithLoopableIteratorOptions(
        gronos.WithExtraCancel(cleanup),
    ),
)
```

## Advanced Use Cases

### Error Handling and Recovery

You can implement sophisticated error handling and recovery strategies:

```go
iterApp := gronos.Iterator(ctx, tasks,
    gronos.WithLoopableIteratorOptions(
        gronos.WithOnError(func(err error) error {
            if errors.Is(err, TemporaryError) {
                log.Printf("Temporary error occurred: %v. Retrying...", err)
                time.Sleep(5 * time.Second)
                return nil // continue iteration
            }
            if errors.Is(err, PermanentError) {
                log.Printf("Permanent error occurred: %v. Stopping iteration.", err)
                return gronos.ErrLoopCritical
            }
            return err // propagate other errors
        }),
    ),
)
```

### Conditional Loop Termination

You can implement complex conditions for terminating the loop:

```go
var successCount, failureCount int
iterApp := gronos.Iterator(ctx, tasks,
    gronos.WithLoopableIteratorOptions(
        gronos.WithShouldStop(func(err error) bool {
            if err != nil {
                failureCount++
            } else {
                successCount++
            }
            return failureCount > 5 || successCount >= 100
        }),
    ),
)
```

### State Management Between Iterations

You can manage state between iterations using closure variables:

```go
var state = make(map[string]interface{})
tasks := []gronos.CancellableTask{
    func(ctx context.Context) error {
        state["timestamp"] = time.Now()
        return nil
    },
    func(ctx context.Context) error {
        fmt.Printf("Time since last iteration: %v\n", time.Since(state["timestamp"].(time.Time)))
        return nil
    },
}

iterApp := gronos.Iterator(ctx, tasks)
```

### Dynamic Task Modification

You can modify the task list dynamically based on certain conditions:

```go
var dynamicTasks []gronos.CancellableTask
iterApp := gronos.Iterator(ctx, dynamicTasks,
    gronos.WithLoopableIteratorOptions(
        gronos.WithBeforeLoop(func() error {
            if someCondition() {
                dynamicTasks = append(dynamicTasks, newTask)
            }
            return nil
        }),
    ),
)
```

### Timed Iterations

You can implement timed iterations:

```go
iterApp := gronos.Iterator(ctx, tasks,
    gronos.WithLoopableIteratorOptions(
        gronos.WithBeforeLoop(func() error {
            time.Sleep(time.Minute) // Wait for 1 minute between iterations
            return nil
        }),
    ),
)
```

## Best Practices

1. **Keep tasks atomic**: Each task in the iterator should be a self-contained unit of work.
2. **Handle context cancellation**: Each task should respect context cancellation for proper shutdown.
3. **Use appropriate error handling**: Implement error handling that fits your application's needs.
4. **Avoid shared mutable state**: If tasks need to share state, use thread-safe methods or channels.
5. **Log judiciously**: Log important events and errors, but avoid excessive logging in tight loops.

## Common Pitfalls

1. **Infinite loops**: Ensure you have a proper termination condition, either through `WithShouldStop` or context cancellation.
2. **Resource leaks**: Make sure to clean up resources in `WithAfterLoop` or use `defer` statements in your tasks.
3. **Blocking indefinitely**: Avoid operations that can block indefinitely; always use timeouts or context deadlines.
4. **Ignoring errors**: Don't ignore errors unless you have a good reason; unhandled errors can lead to unexpected behavior.
5. **Over-complicated logic**: Keep your iterator logic as simple as possible. If it becomes too complex, consider breaking it into multiple iterators or rethinking your approach.

By mastering the Gronos Iterator and its options, you can implement complex, resilient, and flexible task sequencing in your applications. Remember to always consider the specific needs of your application when deciding how to configure and use the Iterator.