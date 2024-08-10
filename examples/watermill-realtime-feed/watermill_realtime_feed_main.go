package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"time"

	"github.com/ThreeDotsLabs/watermill"
	"github.com/ThreeDotsLabs/watermill/message"
	"github.com/ThreeDotsLabs/watermill/message/router/middleware"
	"github.com/ThreeDotsLabs/watermill/message/router/plugin"
	"github.com/ThreeDotsLabs/watermill/pubsub/gochannel"
	"github.com/davidroman0O/gronos"
	watermillextension "github.com/davidroman0O/gronos/watermill"
)

func preparePublisherSubscriber(ctx context.Context, shutdown <-chan struct{}) error {
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

	return nil
}

func prepareRouter(ctx context.Context, shutdown <-chan struct{}) error {
	bus, err := gronos.UseBusWait(ctx)
	if err != nil {
		return err
	}

	router, err := message.NewRouter(message.RouterConfig{}, watermill.NewStdLogger(false, false))

	if err != nil {
		return err
	}

	<-bus(func() (<-chan struct{}, gronos.Message) {
		return watermillextension.MsgAddRouter("router", router)
	})

	return nil
}

func prepareRouterPluginsMiddlewares(ctx context.Context, shutdown <-chan struct{}) error {
	busConfirm, err := gronos.UseBusConfirm(ctx)
	if err != nil {
		return err
	}
	bus, err := gronos.UseBusWait(ctx)
	if err != nil {
		return err
	}

	for !<-busConfirm(func() (<-chan bool, gronos.Message) {
		return watermillextension.MsgHasPublisher("pubsub")
	}) {
		time.Sleep(100 * time.Millisecond)
	}

	publisher, err := watermillextension.UsePublisher(ctx, "pubsub")
	if err != nil {
		return err
	}

	poisonQueue, err := middleware.PoisonQueue(publisher, "poison_queue")
	if err != nil {
		panic(err)
	}

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
}

func signalReadyness(ctx context.Context, shutdown <-chan struct{}) error {
	busConfirm, err := gronos.UseBusConfirm(ctx)
	if err != nil {
		panic(err)
	}

	for !<-busConfirm(func() (<-chan bool, gronos.Message) {
		return watermillextension.MsgHasPublisher("pubsub")
	}) {
		time.Sleep(100 * time.Millisecond)
	}

	for !<-busConfirm(func() (<-chan bool, gronos.Message) {
		return watermillextension.MsgHasSubscriber("pubsub")
	}) {
		time.Sleep(100 * time.Millisecond)
	}

	for !<-busConfirm(func() (<-chan bool, gronos.Message) {
		return watermillextension.MsgHasRouter("router")
	}) {
		time.Sleep(100 * time.Millisecond)
	}

	return nil
}

func main() {
	logger := watermill.NewStdLogger(false, false)
	watermillExt := watermillextension.New[string](
		logger,
	)

	ctx := context.Background()
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	g, cerrs := gronos.New[string](
		ctx,
		map[string]gronos.RuntimeApplication{
			"setup-pubsub":                     preparePublisherSubscriber,
			"setup-router":                     prepareRouter,
			"setup-router-plugins-middlewares": prepareRouterPluginsMiddlewares,
			"ready":                            signalReadyness,
		},
		gronos.WithExtension[string](watermillExt))

	go func() {
		for e := range cerrs {
			fmt.Println(e)
		}
	}()

	// until readyness is not confirmed
	for !g.IsComplete("ready") {
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

	go func() {
		c := make(chan os.Signal, 1)
		signal.Notify(c, os.Interrupt)
		<-c
		g.Shutdown()
	}()

	g.Wait()

}
