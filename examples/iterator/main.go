package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/davidroman0O/gronos"
)

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	g := gronos.New[string](ctx, nil)

	steps := []gronos.CancellableTask{
		func(ctx context.Context) error {
			// Step 1 logic
			fmt.Println("Step 1")
			return nil
		},
		func(ctx context.Context) error {
			// Step 2 logic
			fmt.Println("Step 2")
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
		fmt.Println("Extra cancel")
	}

	err := g.Add("asideWorker", gronos.Worker(
		time.Second/2, gronos.ManagedTimeline, func(ctx context.Context) error {
			fmt.Println("work work work")
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

	go func() {
		<-time.After(time.Second * 2)
		// extraCancel()
		// cancel()
		// <-time.After(time.Second * 1)
		g.Shutdown() // should not trigger cancellations
	}()

	g.Wait()
}