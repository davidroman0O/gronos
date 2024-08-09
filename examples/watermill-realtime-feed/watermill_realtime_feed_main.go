package main

import (
	"context"
	"fmt"
	"time"

	"github.com/ThreeDotsLabs/watermill"
	"github.com/ThreeDotsLabs/watermill/message"
	"github.com/ThreeDotsLabs/watermill/message/router/middleware"
	"github.com/ThreeDotsLabs/watermill/message/router/plugin"
	"github.com/ThreeDotsLabs/watermill/pubsub/gochannel"
	"github.com/davidroman0O/gronos"
	watermillextension "github.com/davidroman0O/gronos/watermill"
)

func main() {
	logger := watermill.NewStdLogger(false, false)
	watermillExt := watermillextension.New[string](
		logger,
	)

	g, cerrs := gronos.New[string](
		context.Background(),
		map[string]gronos.RuntimeApplication{
			"setup": func(ctx context.Context, shutdown <-chan struct{}) error {

				bus, err := gronos.UseBusWait(ctx)
				if err != nil {
					return err
				}

				pubSub := gochannel.NewGoChannel(gochannel.Config{}, watermill.NewStdLogger(false, false))

				<-bus(func() (<-chan struct{}, gronos.Message) {
					return watermillextension.MsgAddPublisher("pubsub", pubSub)
				})

				<-bus(func() (<-chan struct{}, gronos.Message) {
					return watermillextension.MsgAddSubscriber("pubsub", pubSub)
				})

				publisher, err := watermillextension.UsePublisher(ctx, "pubsub")
				if err != nil {
					return err
				}

				router, err := message.NewRouter(message.RouterConfig{}, watermill.NewStdLogger(false, false))
				if err != nil {
					return err
				}

				poisonQueue, err := middleware.PoisonQueue(publisher, "poison_queue")
				if err != nil {
					panic(err)
				}

				<-bus(func() (<-chan struct{}, gronos.Message) {
					return watermillextension.MsgAddRouter("router", router)
				})

				<-bus(func() (<-chan struct{}, gronos.Message) {
					return watermillextension.MsgAddMiddlewares(
						"router",
						middleware.Recoverer,
						middleware.NewThrottle(10, time.Second).Middleware,
						poisonQueue,
						middleware.CorrelationID,
						middleware.Retry{
							MaxRetries:      1,
							InitialInterval: time.Millisecond * 10,
						}.Middleware,
					)
				})

				<-bus(func() (<-chan struct{}, gronos.Message) {
					return watermillextension.MsgAddPlugins(
						"router",
						plugin.SignalsHandler,
					)
				})

				return nil
			},
		},
		gronos.WithExtension[string](watermillExt))

	go func() {
		for e := range cerrs {
			fmt.Println(e)
		}
	}()

	for !g.IsComplete("setup") {
		time.Sleep(100 * time.Millisecond)
	}

	<-g.Add(
		"producer",
		func(ctx context.Context, shutdown <-chan struct{}) error {

			return nil
		},
	)

	<-g.Add(
		"consumer",
		func(ctx context.Context, shutdown <-chan struct{}) error {
			bus, err := gronos.UseBusWait(ctx)
			if err != nil {
				return err
			}

			<-bus(func() (<-chan struct{}, gronos.Message) {
				return watermillextension.MsgAddHandler(
					"router",
					"posts_counter",
					"posts_published",
					"pubsub",
					"posts_count",
					"pubsub",
					func(msg *message.Message) ([]*message.Message, error) {

						return nil, nil
					},
				)
			})

			select {
			case <-shutdown:
				return nil
			case <-ctx.Done():
				return nil
			default:
				return nil
			}
		},
	)

	g.Wait()

}
