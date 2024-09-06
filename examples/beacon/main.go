package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"

	"github.com/davidroman0O/gronos"
	"github.com/davidroman0O/gronos/etcd"
)

/// even the follower doesn't works
/// Probably that the etcd server is not running in the background for the leaders, also seems that the client is not connecting to the etcd server....

func main() {
	ctx, cancel := context.WithCancel(context.Background())

	opts, err := gronos.NewOptionsBuilder(etcd.ModeBeacon).
		WithName("beacon").
		WithRemoveDataDir().
		WithBeacon("localhost:5000").
		WithEndpoints([]string{"localhost:2379"}).
		Build()

	if err != nil {
		panic(fmt.Errorf("Error creating new options for leader: %v", err))
	}

	g, err := gronos.New[string](opts)
	if err != nil {
		panic(fmt.Errorf("Error creating new gronos for leader: %v", err))
	}

	go func() {
		c := make(chan os.Signal, 1)
		signal.Notify(c, os.Interrupt)
		<-c
		cancel()
	}()

	if err = g.Run(ctx); err != nil && err != context.Canceled {
		panic(fmt.Errorf("Error running gronos leader: %v", err))
	}
}
