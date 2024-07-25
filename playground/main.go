package main

import (
	"context"
	"fmt"
	"time"

	"github.com/davidroman0O/gronos/nonos"
)

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	nono := nonos.New[string](ctx, map[string]nonos.RuntimeApplication{
		"app1": func(ctx context.Context, shutdown <-chan struct{}) error {
			fmt.Println("app1 started")

			com, err := nonos.UseBus(ctx)
			if err != nil {
				return err
			}

			go func() {
				<-time.After(time.Second * 2)

				com <- nonos.Add[string]{
					Key: "worker3",
					App: nonos.Worker(time.Second, nonos.ManagedTimeline, func(ctx context.Context) error {
						fmt.Println("worker3 tick")
						return nil
					}),
				}

				com <- nonos.DeadLetter[string]{Key: "app1", Reason: fmt.Errorf("kym")}
			}()

			select {
			case <-shutdown:
				// Handle successful start
			case <-time.After(time.Second * 10): // Adjust the timeout as necessary
				fmt.Println("Timeout waiting for start signal")
			}
			fmt.Println("app1 ended")
			return nil
		},
		"worker1": nonos.Worker(
			time.Second/4,
			nonos.ManagedTimeline,
			func(ctx context.Context) error {
				fmt.Println("worker1 tick")
				com, err := nonos.UseBus(ctx)
				if err != nil {
					return err
				}
				go func() {
					<-time.After(time.Second * 3)
					com <- nonos.DeadLetter[string]{Key: "worker2", Reason: fmt.Errorf("kym")}
				}()
				go func() {
					<-time.After(time.Second * 5)
					com <- nonos.DeadLetter[string]{Key: "worker3", Reason: fmt.Errorf("kym")}
				}()
				<-ctx.Done()
				return nil
			}),
		"worker2": nonos.Worker(
			time.Second/4,
			nonos.ManagedTimeline,
			func(ctx context.Context) error {
				fmt.Println("worker2 tick")
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
