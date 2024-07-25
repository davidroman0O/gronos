```go

package main 

import (
	"fmt"
	"github.com/davidroman0O/gronos"
)

func ChildApp(ctx context.Context, shutdown chan struct{}) error {
	<-shutdown
	return nil
}

func main() {
	am := NewAppManager[string](5 * time.Second) // shutdown timeout `timeout time.Duration`

	shutdownChan, errChan := am.Run(map[string]gronos.RuntimeApplication{
		"app1": func(ctx context.Context, shutdown chan struct{}) error {

			err := UseAdd(ctx, am.useAddChan, "child", ChildApp)
			if err != nil {
				return err
			}
			
			state, err := UseWait(ctx, "child", StateStarted)
			if err != nil {
				return err
			}
			<-state
			
			time.Sleep(50 * time.Millisecond)
			err = <-UseShutdown(ctx, "child")
			if err != nil {
				return err
			}

			time.Sleep(50 * time.Millisecond)
			state, err = <-UseWait(ctx, "child", StateCompleted)
			if err != nil {
				return err
			}
			<-state
			
			
			go func() {
				time.Sleep(1 * time.Second)
				err = <-UseShutdown(ctx, "app1")
				if err != nil {
					return err
				}
			}

			<-shutdown

			return nil
		},
	})

	am.AddApplication("child2", ChildApp)

	go func() {
		state, err := am.WaitApplication("child2", StateStarted)
		if err != nil {
			fmt.Println(err)
			return
		}
		<-state
		am.ShutdownApplication("child2")
	}

	<-shutdownChan // will block until all apps are shutdown or if `close(shutdownChan)`

	fmt.Println(<-errChan)
}

```