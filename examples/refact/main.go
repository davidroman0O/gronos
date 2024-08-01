package main

import (
	"context"
	"time"

	"github.com/charmbracelet/log"
	"github.com/davidroman0O/gronos"
)

func main() {
	ctx := context.Background()
	g := gronos.New[string](ctx, map[string]gronos.RuntimeApplication{
		"app1": func(ctx context.Context, shutdown <-chan struct{}) error {
			log.Info("App1 started")
			defer log.Info("[App1] defer done")
			timer := time.NewTimer(2 * time.Second)
			for {
				select {
				case <-shutdown:
					timer.Stop()
					log.Info("[App1] shutting down")
					return nil
				case <-ctx.Done():
					timer.Stop()
					log.Info("[App1] context done")
					return nil
				case <-timer.C:
					log.Info("[App1] tick")
				}
			}
			return nil
		},
		"app3": func(ctx context.Context, shutdown <-chan struct{}) error {
			log.Info("App3 started")
			defer log.Info("[App3] defer done")
			timer := time.NewTimer(2 * time.Second)
			for {
				select {
				case <-shutdown:
					timer.Stop()
					log.Info("[App3] shutting down")
					return nil
				case <-ctx.Done():
					timer.Stop()
					log.Info("[App3] context done")
					return nil
				case <-timer.C:
					log.Info("[App3] tick")
				}
			}
			return nil
		},
	})

	log.Info("[Example] Starting")
	cerr := g.Start()
	go func() {
		for m := range cerr {
			log.Info("error:", m)
		}
	}()

	log.Info("[Example] Adding App2")
	if started := g.Add("app2", func(ctx context.Context, shutdown <-chan struct{}) error {
		log.Info("[App2] started")
		defer log.Info("[App2] defer done")
		select {
		case <-ctx.Done():
			log.Info("[App2] context done")
		case <-shutdown:
			log.Info("[App2] shutting down")
		}
		return nil
	}); started != nil {
		<-started
	}

	<-time.After(1 * time.Second)
	log.Info("[Example] Shutting down")
	g.Shutdown()
	log.Info("[Example] Waiting")
	g.Wait()
	log.Info("[Example] Done")

}
