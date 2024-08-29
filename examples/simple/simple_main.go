package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"sync/atomic"
	"time"

	"github.com/davidroman0O/gronos"
)

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	nono, cerr := gronos.New[string](
		ctx,
		map[string]gronos.LifecyleFunc{
			"app1": func(ctx context.Context, shutdown <-chan struct{}) error {
				log.Println("app1 started")

				bus, err := gronos.UseBus(ctx)
				if err != nil {
					return err
				}

				go func() {
					<-time.After(time.Second * 1)

					switch bus(
						gronos.NewMessageAddLifecycleFunction(
							"worker3",
							func(ctx context.Context, shutdown <-chan struct{}) error {
								log.Println("worker3 tick")
								return nil
							}), gronos.WithTimeout(time.Second)).(type) {
					case gronos.Success[any]:
						log.Println("worker3 added")
					case gronos.Failure:
						log.Println("worker3 failed to add")
					}

					switch bus(
						gronos.NewMessageForceTerminateShutdown("app1"), gronos.WithTimeout(time.Second)).(type) {
					case gronos.Success[any]:
						log.Println("app1 terminated")
					case gronos.Failure:
						log.Println("app1 failed to terminate")
					}
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

					bus, err := gronos.UseBus(ctx)
					if err != nil {
						return err
					}

					go func() {
						<-time.After(time.Second * 1)
						switch bus(
							gronos.NewMessageForceTerminateShutdown("worker2"), gronos.WithTimeout(time.Second)).(type) {
						case gronos.Success[any]:
							log.Println("worker2 terminated")
						case gronos.Failure:
							log.Println("worker2 failed to terminate")
						}

					}()
					go func() {
						<-time.After(time.Second * 2)
						switch bus(
							gronos.NewMessageForceTerminateShutdown("worker3"), gronos.WithTimeout(time.Second)).(type) {
						case gronos.Success[any]:
							log.Println("worker3 terminated")
						case gronos.Failure:
							log.Println("worker3 failed to terminate")
						}
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
		},
		gronos.WithShutdownBehavior[string](gronos.ShutdownManual),
	)

	go func() {
		for msg := range cerr {
			println(msg.Error())
		}
	}()

	ctrlc := atomic.Bool{}
	timour := atomic.Bool{}

	go func() {
		c := make(chan os.Signal, 1)
		signal.Notify(c, os.Interrupt)
		<-c
		ctrlc.Store(true)
		if !timour.Load() {
			nono.Shutdown()
		}
	}()

	go func() {
		<-time.After(time.Second * 1)
		if !ctrlc.Load() {
			timour.Store(true)
			nono.Shutdown()
		}
	}()

	nono.Wait()
	fmt.Println("done")
}
