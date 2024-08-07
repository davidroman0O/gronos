package main

import (
	"context"
	"fmt"
	"log"
	"sync/atomic"
	"time"

	"github.com/davidroman0O/gronos"
)

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	nono, cerr := gronos.New[string](
		ctx,
		map[string]gronos.RuntimeApplication{
			"app1": func(ctx context.Context, shutdown <-chan struct{}) error {
				log.Println("app1 started")

				com, err := gronos.UseBus(ctx)
				if err != nil {
					return err
				}

				go func() {
					<-time.After(time.Second * 1)

					done, msg := gronos.MsgAddRuntimeApplication("worker3",
						gronos.Worker(time.Second, gronos.ManagedTimeline, func(ctx context.Context) error {
							log.Println("worker3 tick")
							return nil
						}))
					com(msg)
					<-done

					com(gronos.MsgForceTerminateShutdown("app1"))
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
						<-time.After(time.Second * 1)
						com(gronos.MsgForceTerminateShutdown("worker2"))
					}()
					go func() {
						<-time.After(time.Second * 2)
						com(gronos.MsgForceTerminateShutdown("worker3"))
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

	// go func() {
	// 	c := make(chan os.Signal, 1)
	// 	signal.Notify(c, os.Interrupt)
	// 	<-c
	// 	ctrlc.Store(true)
	// 	if !timour.Load() {
	// 		nono.Shutdown()
	// 	}
	// }()

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
