package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"

	"github.com/davidroman0O/gronos"
	"github.com/davidroman0O/gronos/etcd"
)

func main() {
	ctx, cancel := context.WithCancel(context.Background())

	opts, err := gronos.NewOptionsBuilder(etcd.ModeLeaderEmbed).
		WithRemoveDataDir().
		WithPorts(2380, 2379).
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
