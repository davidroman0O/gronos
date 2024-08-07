# Gronos Internal Messaging

Gronos provides an internal messaging system for communication between applications. This document covers the usage of the messaging system and available message types.

## Table of Contents

1. [Using the Messaging System](#using-the-messaging-system)
2. [Available Messages](#available-messages)
3. [Creating Custom Messages](#creating-custom-messages)
4. [Examples](#examples)

## Using the Messaging System

To use the internal messaging system, you need to access the message bus from within a `RuntimeApplication`:

```go
func UseBus(ctx context.Context) (func(m Message) bool, error)
```

Example:

```go
func myApp(ctx context.Context, shutdown <-chan struct{}) error {
    bus, err := gronos.UseBus(ctx)
    if err != nil {
        return err
    }

    // Use the bus to send messages
    // ...

    return nil
}
```

## Available Messages

Gronos provides several predefined message types for common operations:

### MsgAddRuntimeApplication

Adds a new runtime application to Gronos.

```go
func MsgAddRuntimeApplication[K comparable](key K, app RuntimeApplication) (<-chan struct{}, *AddRuntimeApplicationMessage[K])
```

### MsgForceCancelShutdown

Forces the cancellation of a shutdown for a specific application.

```go
func MsgForceCancelShutdown[K comparable](key K, err error) *ForceCancelShutdown[K]
```

### MsgForceTerminateShutdown

Forces the termination of a shutdown for a specific application.

```go
func MsgForceTerminateShutdown[K comparable](key K) *ForceTerminateShutdown[K]
```

## Creating Custom Messages

You can create custom messages by implementing the `Message` interface:

```go
type Message interface{}
```

## Examples

### Adding a New Application

```go
func dynamicAppAdder(ctx context.Context, shutdown <-chan struct{}) error {
    bus, err := gronos.UseBus(ctx)
    if err != nil {
        return err
    }

    newApp := func(ctx context.Context, shutdown <-chan struct{}) error {
        // New application logic
        return nil
    }

    done, msg := gronos.MsgAddRuntimeApplication("newApp", newApp)
    bus(msg)
    <-done // Wait for the application to be added

    return nil
}
```

### Forcing Shutdown Cancellation

```go
func shutdownManager(ctx context.Context, shutdown <-chan struct{}) error {
    bus, err := gronos.UseBus(ctx)
    if err != nil {
        return err
    }

    // Force cancel shutdown for "app1"
    bus(gronos.MsgForceCancelShutdown("app1", errors.New("cancel reason")))

    return nil
}
```

### Forcing Shutdown Termination

```go
func emergencyShutdown(ctx context.Context, shutdown <-chan struct{}) error {
    bus, err := gronos.UseBus(ctx)
    if err != nil {
        return err
    }

    // Force terminate shutdown for "app2"
    bus(gronos.MsgForceTerminateShutdown("app2"))

    return nil
}
```

These examples demonstrate how to use the internal messaging system to dynamically add applications, force cancel shutdowns, and force terminate shutdowns for specific applications within the Gronos ecosystem.