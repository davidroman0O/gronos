package gronos

import (
	"context"
	"fmt"
	"time"

	"github.com/charmbracelet/log"
	"github.com/heimdalr/dag"
)

// run is the main loop of the gronos instance, handling messages and managing applications.
func (g *gronos[K]) run(errChan chan<- error) {

	log.Debug("[Gronos] Entering run method")
	defer log.Debug("[Gronos] Exiting run method")

	defer func() {
		// Apply extensions' OnStop hooks
		for _, ext := range g.extensions {
			if err := ext.OnStop(g.ctx, errChan); err != nil {
				errChan <- fmt.Errorf("extension error on stop: %w", err)
			}
		}
		g.sendMessage(g.getSystemMetadata(), MsgDestroy[K]())
	}()

	dag := dag.NewDAG()

	state := &gronosState[K]{
		// dag with weights
		graph:   dag,
		mkeys:   &GMap[K, K]{},
		mapp:    &GMap[K, LifecyleFunc]{},
		mctx:    &GMap[K, context.Context]{},
		mcom:    &GMap[K, chan *MessagePayload[K]]{},
		mret:    &GMap[K, uint]{},
		mshu:    &GMap[K, chan struct{}]{},
		mali:    &GMap[K, bool]{},
		mrea:    &GMap[K, error]{},
		mcloser: &GMap[K, func()]{},
		mcancel: &GMap[K, func()]{},
		mstatus: &GMap[K, StatusState]{},
		mdone:   &GMap[K, chan struct{}]{},
	}

	// Prepare the graph
	state.rootKey = g.getRootKey()

	var err error
	if state.rootVertex, err = state.graph.AddVertex(NewLifecycleVertexData(state.rootKey)); err != nil {
		errChan <- fmt.Errorf("error adding root vertex: %w", err)
		return
	}

	g.startTime = time.Now()

	// global shutdown or cancellation detection
	go func() {
		select {
		case <-g.ctx.Done():
			log.Debug("[Gronos] Context cancelled, initiating shutdown")
			g.sendMessage(g.getSystemMetadata(), MsgInitiateContextCancellation[K]())
		case <-g.shutdownChan:
			log.Debug("[Gronos] Shutdown initiated, initiating shutdown")
			g.sendMessage(g.getSystemMetadata(), MsgInitiateShutdown[K]())
		}
	}()

	for m := range g.publicChn {
		if err := g.handleMessage(state, m); err != nil {
			errChan <- err
		}
	}
	log.Debug("[Gronos] Communication channel closed")
}
