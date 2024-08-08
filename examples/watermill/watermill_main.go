package main

import (
	"context"
	"fmt"
	"time"

	"github.com/ThreeDotsLabs/watermill"
	"github.com/ThreeDotsLabs/watermill/message"
	"github.com/ThreeDotsLabs/watermill/pubsub/gochannel"
	"github.com/davidroman0O/gronos"
	watermillext "github.com/davidroman0O/gronos/watermill"
)

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	watermillMiddleware := watermillext.NewWatermillMiddleware[string](watermill.NewStdLogger(true, true))

	g, errChan := gronos.New[string](ctx, map[string]gronos.RuntimeApplication{
		"setup": setupApp,
	},
		gronos.WithShutdownBehavior[string](gronos.ShutdownAutomatic),
		gronos.WithGracePeriod[string](2*time.Second),
		gronos.WithMinRuntime[string](5*time.Second),
		gronos.WithExtension[string](watermillMiddleware),
	)

	go func() {
		for err := range errChan {
			fmt.Printf("Error: %v\n", err)
		}
	}()

	// Wait for setup to complete
	for !g.IsComplete("setup") {
		time.Sleep(100 * time.Millisecond)
	}

	// Add other applications
	g.Add("publisher", publisherApp)
	g.Add("subscriber", subscriberApp)
	g.Add("router", routerApp)

	// Run for a while
	time.Sleep(5 * time.Second)

	fmt.Println("Shutting down...")

	// Initiate shutdown
	g.Shutdown()

	fmt.Println("Waiting for shutdown to complete...")

	// Wait for shutdown to complete
	g.Wait()
}

func setupApp(ctx context.Context, shutdown <-chan struct{}) error {
	com, err := gronos.UseBus(ctx)
	if err != nil {
		return err
	}

	pubSub := gochannel.NewGoChannel(gochannel.Config{}, watermill.NewStdLogger(false, false))
	doneAddPublisher, msgAddPublisher := watermillext.MsgAddPublisher("pubsub", pubSub)
	com(msgAddPublisher)
	<-doneAddPublisher
	doneAddSubscriber, msgAddSubscriber := watermillext.MsgAddSubscriber("pubsub", pubSub)
	com(msgAddSubscriber)
	<-doneAddSubscriber

	router, err := message.NewRouter(message.RouterConfig{}, watermill.NewStdLogger(false, false))
	if err != nil {
		return err
	}
	doneAddRouter, msgAddRouter := watermillext.MsgAddRouter("router", router)
	com(msgAddRouter)
	<-doneAddRouter

	done, msg := gronos.MsgRequestStatusAsync("setup", gronos.StatusRunning)
	com(msg)
	<-done

	return nil
}

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

	messages, err := subscribe(ctx, "example.processed.topic")
	if err != nil {
		return err
	}

	for {
		select {
		case msg, ok := <-messages:
			if !ok {
				fmt.Println("	Subscriber closed")
				return nil
			}
			fmt.Printf("Received processed message: %s\n", string(msg.Payload))
			msg.Ack()
		case <-ctx.Done():
			return ctx.Err()
		case <-shutdown:
			return nil
		}
	}
}

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
