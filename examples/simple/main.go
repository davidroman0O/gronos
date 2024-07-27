package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/davidroman0O/gronos"
)

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	nono := gronos.New[string](ctx, map[string]gronos.RuntimeApplication{
		"app1": func(ctx context.Context, shutdown <-chan struct{}) error {
			log.Println("app1 started")

			com, err := gronos.UseBus(ctx)
			if err != nil {
				return err
			}

			go func() {
				<-time.After(time.Second * 2)

				com <- gronos.Add[string]{
					Key: "worker3",
					App: gronos.Worker(time.Second, gronos.ManagedTimeline, func(ctx context.Context) error {
						log.Println("worker3 tick")
						return nil
					}),
				}

				com <- gronos.DeadLetter[string]{Key: "app1", Reason: fmt.Errorf("kym")}
			}()

			select {
			case <-shutdown:
				// Handle successful start
			case <-time.After(time.Second * 10): // Adjust the timeout as necessary
				log.Println("Timeout waiting for start signal")
			}
			log.Println("app1 ended")
			return nil
		},
		"worker1": gronos.Worker(
			time.Second/4,
			gronos.ManagedTimeline,
			func(ctx context.Context) error {
				log.Println("worker1 tick")
				com, err := gronos.UseBus(ctx)
				if err != nil {
					return err
				}
				go func() {
					<-time.After(time.Second * 3)
					com <- gronos.DeadLetter[string]{Key: "worker2", Reason: fmt.Errorf("kym")}
				}()
				go func() {
					<-time.After(time.Second * 5)
					com <- gronos.DeadLetter[string]{Key: "worker3", Reason: fmt.Errorf("kym")}
				}()
				<-ctx.Done()
				return nil
			}),
		"worker2": gronos.Worker(
			time.Second/4,
			gronos.ManagedTimeline,
			func(ctx context.Context) error {
				log.Println("worker2 tick")
				return nil
			}),
	})

	e := nono.Start()

	go func() {
		for msg := range e {
			println(msg.Error())
		}
	}()

	go func() {
		<-time.After(time.Second * 5)
		nono.Shutdown()
	}()

	nono.Wait()
}
