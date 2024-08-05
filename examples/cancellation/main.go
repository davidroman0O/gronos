package main

import (
	"context"
	"fmt"
	"time"

	"github.com/davidroman0O/gronos"
)

func main() {
	ctx, cancel := context.WithCancel(context.Background())

	g := gronos.New[string](
		ctx,
		map[string]gronos.RuntimeApplication{
			"app1": func(ctx context.Context, shutdown <-chan struct{}) error {
				select {
				case <-ctx.Done():
					return ctx.Err()
				case <-shutdown:
					return nil
				}
			},
		},
	)

	cer := g.Start()

	go func() {
		for v := range cer {
			fmt.Println(v)
		}
	}()

	<-time.After(time.Second * 5)

	cancel()

	g.Wait()

}
