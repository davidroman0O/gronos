package main

import (
	"context"
	"errors"
	"log"
	"os"
	"os/signal"
	"time"

	"github.com/davidroman0O/gronos"
)

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	g, cerr := gronos.New[string](
		ctx,
		map[string]gronos.RuntimeApplication{
			// "stopper": func(ctx context.Context, shutdown <-chan struct{}) error {
			// 	<-time.After(time.Second * 3)
			// 	log.Println("Shooting others")
			// 	bus, err := gronos.UseBus(ctx)
			// 	if err != nil {
			// 		return err
			// 	}
			// 	bus <- gronos.MsgCancelShutdown("iteratorApp", nil)
			// 	bus <- gronos.MsgTerminateShutdown("asideWorker") // will stop shutdown
			// 	return nil
			// },
		})

	steps := []gronos.CancellableTask{
		func(ctx context.Context) error {
			ptrTime := ctx.Value("key").(*time.Time)
			*ptrTime = time.Now()
			// Step 1 logic
			log.Println("Step 1")
			return nil
		},
		func(ctx context.Context) error {
			// Step 2 logic
			log.Println("Step 2", ctx.Value("key"))
			return nil
		},
		func(ctx context.Context) error {
			// Step 3 logic
			time.Sleep(time.Second * 1)
			return nil
		},
	}

	go func() {
		for m := range cerr {
			log.Println("error:", m)
		}
	}()

	extraCtx, extraCancel := context.WithCancel(context.Background())
	defer extraCancel()

	cancelExtra := func() {
		log.Println("Extra cancel")
	}

	g.Add("asideWorker", gronos.Worker(
		time.Second/2, gronos.ManagedTimeline, func(ctx context.Context) error {
			log.Println("work work work")
			return nil
		}))

	valueOfTime := time.Now()
	extraCtx = context.WithValue(extraCtx, "key", &valueOfTime)

	g.Add("iteratorApp", gronos.Iterator(
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
			gronos.WithBeforeLoop(func(_ context.Context) error {
				log.Println("Starting new iteration")
				return nil
			}),
			gronos.WithAfterLoop(func(_ context.Context) error {
				log.Println("Finished iteration")
				return nil
			}),
		),
	))

	go func() {
		c := make(chan os.Signal, 1)
		signal.Notify(c, os.Interrupt)
		<-c
		g.Shutdown()
	}()

	g.Wait()
	log.Println("Finished")
}
