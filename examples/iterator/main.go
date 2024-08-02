package main

import (
	"context"
	"errors"
	"log"
	"time"

	"github.com/davidroman0O/gronos"
)

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	g := gronos.New[string](ctx, map[string]gronos.RuntimeApplication{
		"stopper": func(ctx context.Context, shutdown <-chan struct{}) error {
			<-time.After(time.Second * 3)
			log.Println("Shooting others")
			bus, err := gronos.UseBus(ctx)
			if err != nil {
				return err
			}
			bus <- gronos.MsgCancelShutdown("iteratorApp", nil)
			bus <- gronos.MsgTerminateShutdown("asideWorker") // will stop shutdown
			return nil
		},
	})

	steps := []gronos.CancellableTask{
		func(ctx context.Context) error {
			// Step 1 logic
			log.Println("Step 1")
			return nil
		},
		func(ctx context.Context) error {
			// Step 2 logic
			log.Println("Step 2")
			return nil
		},
		func(ctx context.Context) error {
			// Step 3 logic
			time.Sleep(time.Second * 1)
			return nil
		},
	}

	extraCtx, extraCancel := context.WithCancel(context.Background())
	defer extraCancel()

	cancelExtra := func() {
		log.Println("Extra cancel")
	}

	err := g.Add("asideWorker", gronos.Worker(
		time.Second/2, gronos.ManagedTimeline, func(ctx context.Context) error {
			log.Println("work work work")
			return nil
		}))
	if err != nil {
		// Handle error
		panic(err)
	}

	err = g.Add("iteratorApp", gronos.Iterator(
		extraCtx,
		steps,
		gronos.WithLoopableIteratorOptions(
			gronos.WithExtraCancel(cancelExtra),
			gronos.WithOnError(func(err error) error {
				if errors.Is(err, gronos.ErrLoopCritical) {
					log.Printf("Critical error: %v", err)
					return nil
				}
				return nil
			}),
			gronos.WithShouldStop(func(err error) bool {
				return err != nil // Stop on any error
			}),
			gronos.WithBeforeLoop(func() error {
				log.Println("Starting new iteration")
				return nil
			}),
			gronos.WithAfterLoop(func() error {
				log.Println("Finished iteration")
				return nil
			}),
		),
	))
	if err != nil {
		// Handle error
		panic(err)
	}

	e := g.Start()

	go func() {
		for msg := range e {
			println(msg.Error())
		}
	}()

	g.Wait()
	log.Println("Finished")
}
