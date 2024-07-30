# Troubleshooting Gronos

This guide addresses common issues you might encounter when using Gronos and provides solutions and workarounds.

## Table of Contents

1. [Application Not Starting](#application-not-starting)
2. [Unexpected Application Termination](#unexpected-application-termination)
3. [Deadlocks and Hangs](#deadlocks-and-hangs)
4. [Memory Leaks](#memory-leaks)
5. [Performance Issues](#performance-issues)
6. [Error Channel Overflow](#error-channel-overflow)
7. [Context Cancellation Not Propagating](#context-cancellation-not-propagating)
8. [Shutdown Taking Too Long](#shutdown-taking-too-long)

## Application Not Starting

**Problem:** An application added to Gronos doesn't seem to start.

**Possible Causes and Solutions:**

1. **Gronos instance not started:** Ensure you've called `g.Start()`.

   ```go
   errChan := g.Start()
   ```

2. **Application panicking on start:** Wrap your application logic in a recover block.

   ```go
   g.Add("myApp", func(ctx context.Context, shutdown <-chan struct{}) error {
       defer func() {
           if r := recover(); r != nil {
               fmt.Printf("Application panicked: %v\n", r)
           }
       }()
       // Your application logic here
       return nil
   })
   ```

3. **Context cancelled before application starts:** Check if the context is already cancelled.

   ```go
   if ctx.Err() != nil {
       return fmt.Errorf("context already cancelled: %w", ctx.Err())
   }
   ```

## Unexpected Application Termination

**Problem:** Applications are terminating unexpectedly.

**Possible Causes and Solutions:**

1. **Unhandled errors:** Ensure all errors are properly handled and logged.

   ```go
   if err := someOperation(); err != nil {
       log.Printf("Error in someOperation: %v", err)
       return err
   }
   ```

2. **Ignoring shutdown signal:** Make sure your application respects the shutdown channel.

   ```go
   select {
   case <-shutdown:
       // Perform cleanup
       return nil
   default:
       // Continue normal operation
   }
   ```

3. **Panics in goroutines:** Use `recover()` in all goroutines.

   ```go
   go func() {
       defer func() {
           if r := recover(); r != nil {
               log.Printf("Goroutine panicked: %v", r)
           }
       }()
       // Goroutine logic
   }()
   ```

## Deadlocks and Hangs

**Problem:** The application seems to hang or deadlock.

**Possible Causes and Solutions:**

1. **Blocking operations without context:** Always use context with timeout for blocking operations.

   ```go
   ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
   defer cancel()
   if err := blockingOperation(ctx); err != nil {
       return err
   }
   ```

2. **Circular dependencies between applications:** Refactor your applications to remove circular dependencies.

3. **Misuse of channels:** Ensure proper channel usage, avoid sending on closed channels.

   ```go
   select {
   case ch <- value:
       // Sent successfully
   default:
       // Channel is full or closed
   }
   ```

## Memory Leaks

**Problem:** The application's memory usage keeps increasing over time.

**Possible Causes and Solutions:**

1. **Goroutines not terminating:** Ensure all goroutines have a way to terminate.

   ```go
   go func() {
       defer fmt.Println("Goroutine terminated")
       for {
           select {
           case <-ctx.Done():
               return
           default:
               // Work
           }
       }
   }()
   ```

2. **Large objects not being garbage collected:** Use `runtime.GC()` and `debug.FreeOSMemory()` for debugging.

   ```go
   import (
       "runtime"
       "runtime/debug"
   )

   // After suspected memory leak
   runtime.GC()
   debug.FreeOSMemory()
   ```

3. **Inefficient use of `sync.Pool`:** Ensure objects are removed from the pool when no longer needed.

## Performance Issues

**Problem:** The application is running slower than expected.

**Possible Causes and Solutions:**

1. **Too many goroutines:** Limit the number of concurrent goroutines.

   ```go
   sem := make(chan struct{}, maxConcurrency)
   for i := 0; i < tasks; i++ {
       sem <- struct{}{}
       go func() {
           defer func() { <-sem }()
           // Task logic
       }()
   }
   ```

2. **Inefficient algorithms:** Profile your code and optimize hot paths.

   ```go
   import "runtime/pprof"

   pprof.StartCPUProfile(outputFile)
   defer pprof.StopCPUProfile()
   ```

3. **Excessive logging:** Reduce log verbosity in performance-critical sections.

## Error Channel Overflow

**Problem:** The error channel is getting full, leading to blocked goroutines.

**Possible Causes and Solutions:**

1. **Not consuming errors fast enough:** Ensure you're reading from the error channel consistently.

   ```go
   go func() {
       for err := range errChan {
           log.Printf("Error: %v", err)
       }
   }()
   ```

2. **Too many errors being generated:** Implement rate limiting for error reporting.

   ```go
   var errorCount int32
   // In your error handling logic
   if atomic.AddInt32(&errorCount, 1) > maxErrorRate {
       // Skip reporting this error
       atomic.AddInt32(&errorCount, -1)
       return
   }
   // Report error
   ```

## Context Cancellation Not Propagating

**Problem:** Applications not stopping when the main context is cancelled.

**Possible Causes and Solutions:**

1. **Not checking context in loops:** Always check for context cancellation in long-running loops.

   ```go
   for {
       select {
       case <-ctx.Done():
           return ctx.Err()
       default:
           // Work
       }
   }
   ```

2. **Using background context instead of passed context:** Always use the context passed to the application.

   ```go
   func myApp(ctx context.Context, shutdown <-chan struct{}) error {
       // Use ctx, not context.Background()
   }
   ```

## Shutdown Taking Too Long

**Problem:** The application takes too long to shut down.

**Possible Causes and Solutions:**

1. **Long-running operations not respecting shutdown:** Implement timeout for shutdown operations.

   ```go
   func (a *App) Shutdown(ctx context.Context) error {
       done := make(chan struct{})
       go func() {
           // Shutdown logic
           close(done)
       }()
       select {
       case <-done:
           return nil
       case <-ctx.Done():
           return ctx.Err()
       }
   }
   ```

2. **Too many applications to shut down sequentially:** Implement parallel shutdown for independent applications.

   ```go
   var wg sync.WaitGroup
   for _, app := range apps {
       wg.Add(1)
       go func(a *App) {
           defer wg.Done()
           a.Shutdown(ctx)
       }(app)
   }
   wg.Wait()
   ```

By addressing these common issues, you can improve the reliability and performance of your Gronos-based applications. For more advanced usage patterns and best practices, refer to the [Advanced Usage](advanced-usage.md) and [Best Practices](best-practices.md) sections.