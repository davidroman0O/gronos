package main

import (
	"context"
	"time"

	"github.com/charmbracelet/log"
	"github.com/davidroman0O/gronos"
)

func main() {
	ctx := context.Background()
	g, cerr := gronos.New[string](ctx, map[string]gronos.LifecyleFunc{
		"app1": func(ctx context.Context, shutdown <-chan struct{}) error {
			log.Debug("App1 started")
			defer log.Debug("[App1] defer done")
			timer := time.NewTimer(2 * time.Second)
			for {
				select {
				case <-shutdown:
					timer.Stop()
					log.Debug("[App1] shutting down")
					return nil
				case <-ctx.Done():
					timer.Stop()
					log.Debug("[App1] context done")
					return nil
				case <-timer.C:
					log.Debug("[App1] tick")
				}
			}
			return nil
		},
		"app3": func(ctx context.Context, shutdown <-chan struct{}) error {
			log.Debug("App3 started")
			defer log.Debug("[App3] defer done")
			timer := time.NewTimer(2 * time.Second)
			for {
				select {
				case <-shutdown:
					timer.Stop()
					log.Debug("[App3] shutting down")
					return nil
				case <-ctx.Done():
					timer.Stop()
					log.Debug("[App3] context done")
					return nil
				case <-timer.C:
					log.Debug("[App3] tick")
				}
			}
			return nil
		},
	})

	log.Debug("[Example] Starting")

	go func() {
		for m := range cerr {
			log.Debug("error:", m)
		}
	}()

	log.Debug("[Example] Adding App2")
	if started := g.Add("app2", func(ctx context.Context, shutdown <-chan struct{}) error {
		log.Debug("[App2] started")
		defer log.Debug("[App2] defer done")
		select {
		case <-ctx.Done():
			log.Debug("[App2] context done")
		case <-shutdown:
			log.Debug("[App2] shutting down")
		}
		return nil
	}); started != nil {
		<-started
	}

	<-time.After(1 * time.Second)
	log.Debug("[Example] Shutting down")
	g.Shutdown()
	log.Debug("[Example] Waiting")
	g.Wait()
	log.Debug("[Example] Done")

}
