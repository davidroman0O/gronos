package gronos

import (
	"context"
	"log/slog"
	"time"
)

type contextKey string

const appManagerKey contextKey = "appManager"
const currentAppKey contextKey = "currentApp"

func UseAdd[Key RuntimeKey](ctx context.Context, addChan chan<- AppManagerMsg[Key], name Key, app RuntimeApplication) {
	var parent Key
	if parentValue := ctx.Value(currentAppKey); parentValue != nil {
		if p, ok := parentValue.(Key); ok {
			parent = p
		}
	}

	select {
	case addChan <- AppManagerMsg[Key]{name: name, app: app, parent: parent}:
	case <-ctx.Done():
	}
}

func UseWait[Key RuntimeKey](ctx context.Context, am *AppManager[Key], name Key, state AppState) <-chan struct{} {
	waitChan := make(chan struct{})
	go func() {
		defer close(waitChan)
		for {
			select {
			case <-ctx.Done():
				return
			default:
			}

			app, loaded := am.apps.Load(name)
			if !loaded {
				return
			}
			appData := app.(struct {
				app        RuntimeApplication
				cancelFunc context.CancelFunc
				shutdownCh chan struct{}
			})

			switch state {
			case StateStarted:
				if appData.shutdownCh != nil {
					return
				}
			case StateRunning:
				if appData.shutdownCh != nil && am.ctx.Err() == nil {
					return
				}
			case StateCompleted:
				select {
				case <-appData.shutdownCh:
					return
				default:
				}
			case StateError:
				select {
				case err := <-am.errChan:
					if err != nil {
						return
					}
				default:
				}
			}

			time.Sleep(50 * time.Millisecond)
		}
	}()
	return waitChan
}

func UseShutdown[Key RuntimeKey](ctx context.Context, shutdownChan chan<- Key, name Key) {
	select {
	case shutdownChan <- name:
	case <-ctx.Done():
		slog.Error("Failed to send shutdown signal", "name", name, "error", ctx.Err())
	}
}
