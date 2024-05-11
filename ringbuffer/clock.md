The `Clock` is a simple ticker that allows you to control when and how to trigger multiple `Ticker` instances. It supports three execution modes:

1. **NonBlocking**: In this mode, the `Ticker.Tick()` method is executed in a separate goroutine without waiting for the previous execution to finish. This mode is useful when you don't want the ticker execution to block the main goroutine.

2. **ManagedTimeline**: In this mode, the `Ticker.Tick()` method is executed at the specified interval, even if the previous execution took longer than the interval. If the previous execution took longer than the interval, the next execution will happen as soon as possible, without skipping any executions.

3. **BestEffort**: In this mode, the `Ticker.Tick()` method is executed at the specified interval. However, if the previous execution took longer than the interval, the current execution will be skipped to avoid overlapping with the previous execution. This mode ensures that there is always a minimum interval between executions, but it may skip some executions if the previous execution takes too long.

**Examples**

Here's an example of how to create a `Clock` and add `Ticker` instances with different execution modes:

```go
// Create a new Clock with a 1-second interval
clock := NewClock(time.Second)

// Add a non-blocking ticker
clock.Add(myNonBlockingTicker, NonBlocking)

// Add a ticker with a managed timeline
clock.Add(myManagedTimelineTicker, ManagedTimeline)

// Add a best-effort ticker
clock.Add(myBestEffortTicker, BestEffort)
```

In this example, `myNonBlockingTicker` will be executed in a separate goroutine without waiting for the previous execution to finish. `myManagedTimelineTicker` will be executed at the specified 1-second interval, even if the previous execution took longer than 1 second. `myBestEffortTicker` will be executed at the 1-second interval, but if the previous execution took longer than 1 second, the current execution will be skipped.

You can also use the `WithPriority` and `WithDynamicInterval` option functions to configure the `Ticker` instances:

```go
// Add a ticker with a priority of 2 and a dynamic interval
count := 0
clock.Add(myDynamicTicker, ManagedTimeline, WithPriority(2), WithDynamicInterval(func(elapsedTime time.Duration) time.Duration {
    count++
    if count%5 == 0 {
        return 2 * time.Second
    }
    return time.Second
}))
```

In this example, `myDynamicTicker` is added with a priority of 2 and a dynamic interval that doubles every 5 seconds based on the elapsed time since the last execution.

**Choosing the Right Execution Mode**

When choosing the execution mode for your `Ticker` instances, consider the following:

- Use `NonBlocking` mode when you don't want the ticker execution to block the main goroutine, and you don't care about the exact timing of the executions.
- Use `ManagedTimeline` mode when you want the ticker to execute at the specified interval, even if the previous execution took longer than the interval. This mode is useful when you need to catch up on missed executions.
- Use `BestEffort` mode when you want the ticker to execute at the specified interval, but you don't want executions to overlap. This mode is useful when you want to maintain a minimum interval between executions, but it's acceptable to skip some executions if necessary.

Remember that the execution mode affects the timing and behavior of your `Ticker` instances, so choose the appropriate mode based on your requirements and the nature of the tasks being executed.
