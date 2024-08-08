# Gronos Watermill Integration

Gronos provides seamless integration with the Watermill library, allowing you to incorporate event-driven architecture and message routing into your Gronos applications. This document covers the usage and configuration of Watermill integration in Gronos.

## Table of Contents

1. [Overview](#overview)
2. [Setting Up Watermill Middleware](#setting-up-watermill-middleware)
3. [Adding Publishers and Subscribers](#adding-publishers-and-subscribers)
4. [Using Watermill Router](#using-watermill-router)
5. [Available Messages](#available-messages)
6. [Examples](#examples)

## Overview

The Watermill integration in Gronos allows you to:

- Use Watermill's pub/sub functionality within Gronos applications
- Add publishers and subscribers dynamically
- Utilize Watermill's router for advanced message handling

## Setting Up Watermill Middleware

To use Watermill with Gronos, you need to set up the Watermill middleware:

```go
import (
    "github.com/ThreeDotsLabs/watermill"
    "github.com/davidroman0O/gronos"
    watermillext "github.com/davidroman0O/gronos/watermill"
)

watermillMiddleware := watermillext.NewWatermillMiddleware[string](watermill.NewStdLogger(true, true))

g, errChan := gronos.New[string](ctx, map[string]gronos.RuntimeApplication{
    "setup": setupApp,
},
    gronos.WithExtension[string](watermillMiddleware),
)
```

## Adding Publishers and Subscribers

You can add publishers and subscribers using the Gronos message bus:

```go
func setupApp(ctx context.Context, shutdown <-chan struct{}) error {
    com, err := gronos.UseBus(ctx)
    if err != nil {
        return err
    }

    pubSub := gochannel.NewGoChannel(gochannel.Config{}, watermill.NewStdLogger(false, false))

    doneAddPublisher, msgAddPublisher := watermillext.MsgAddPublisher("pubsub", pubSub)
    come(msgAddPublisher)
    <-doneAddPublisher

    doneAddSubscriber, msgAddSubscriber := watermillext.MsgAddSubscriber("pubsub", pubSub)
    come(msgAddSubscriber)
    <-doneAddSubscriber

    return nil
}
```

## Using Watermill Router

You can add a Watermill router to your Gronos application:

```go
router, err := message.NewRouter(message.RouterConfig{}, watermill.NewStdLogger(false, false))
if err != nil {
    return err
}

done, msg := watermillext.MsgAddRouter("router", router)
com(msg)
<-done
```

## Available Messages

Gronos provides several Watermill-specific messages:

- `MsgAddPublisher`: Adds a new Watermill publisher
- `MsgAddSubscriber`: Adds a new Watermill subscriber
- `MsgAddRouter`: Adds a Watermill router
- `MsgAddHandler`: Adds a handler to a Watermill router

## Examples

### Publishing and Subscribing

```go
func publisherApp(ctx context.Context, shutdown <-chan struct{}) error {
    publish, err := watermillext.UsePublisher(ctx, "pubsub")
    if err != nil {
        return err
    }

    ticker := time.NewTicker(time.Second)
    defer ticker.Stop()

    for {
        select {
        case <-ticker.C:
            msg := message.NewMessage(watermill.NewUUID(), []byte("Hello, Watermill!"))
            if err := publish("example.topic", msg); err != nil {
                return err
            }
            fmt.Println("Published message")
        case <-ctx.Done():
            return ctx.Err()
        case <-shutdown:
            return nil
        }
    }
}

func subscriberApp(ctx context.Context, shutdown <-chan struct{}) error {
    subscribe, err := watermillext.UseSubscriber(ctx, "pubsub")
    if err != nil {
        return err
    }

    messages, err := subscribe(ctx, "example.topic")
    if err != nil {
        return err
    }

    for {
        select {
        case msg := <-messages:
            fmt.Printf("Received message: %s\n", string(msg.Payload))
            msg.Ack()
        case <-ctx.Done():
            return ctx.Err()
        case <-shutdown:
            return nil
        }
    }
}
```

### Using Router

```go
func routerApp(ctx context.Context, shutdown <-chan struct{}) error {
    com, err := gronos.UseBus(ctx)
    if err != nil {
        return err
    }

    done, msg := watermillext.MsgAddHandler(
            "router",
            "example-handler",
            "example.topic",
            "example.processed.topic",
            func(msg *message.Message) ([]*message.Message, error) {
                fmt.Printf("Processing message: %s\n", string(msg.Payload))
                processedMsg := message.NewMessage(watermill.NewUUID(), []byte("Processed: "+string(msg.Payload)))
                return message.Messages{processedMsg}, nil
            },
        )
    com(msg)
    <-done

    <-shutdown
    return nil
}
```

This example demonstrates how to set up a Watermill router with a handler in a Gronos application.

For more information on Watermill's features and capabilities, please refer to the [Watermill documentation](https://watermill.io/docs/).

TODO: 
- if you use custom generics, you need to specify it on all UsePublisher and UseSubscriber