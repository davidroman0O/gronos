package gronos

import (
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"github.com/charmbracelet/log"
)

type MessageInitiateContextCancellation[K Primitive] struct {
	FutureMessage[Void, Void]
}

var messageInitiateContextCancellationPoolInited bool = false
var messageInitiateContextCancellationPool sync.Pool

func NewMessageInitiateContextCancellation[K Primitive]() *MessageInitiateContextCancellation[K] {
	if !messageInitiateContextCancellationPoolInited {
		messageInitiateContextCancellationPoolInited = true
		messageInitiateContextCancellationPool = sync.Pool{
			New: func() any {
				return &MessageInitiateContextCancellation[K]{}
			},
		}
	}
	return &MessageInitiateContextCancellation[K]{
		FutureMessage: FutureMessage[Void, Void]{
			Data:   Void{},
			Result: NewFuture[Void](),
		},
	}
}

type MessageShutdownProgress[K Primitive] struct {
	FutureMessage[struct{ RemainingApps int }, Void]
}

var messageShutdownProgressPoolInited bool = false
var messageShutdownProgressPool sync.Pool

func NewMessageShutdownProgress[K Primitive](remainingApps int) *MessageShutdownProgress[K] {
	if !messageShutdownProgressPoolInited {
		messageShutdownProgressPoolInited = true
		messageShutdownProgressPool = sync.Pool{
			New: func() any {
				return &MessageShutdownProgress[K]{}
			},
		}
	}
	return &MessageShutdownProgress[K]{
		FutureMessage: FutureMessage[struct{ RemainingApps int }, Void]{
			Data:   struct{ RemainingApps int }{RemainingApps: remainingApps},
			Result: NewFuture[Void](),
		},
	}
}

func (g *gronos[K]) handleShutdownMessage(state *gronosState[K], m *MessagePayload[K]) (error, bool) {
	switch msg := m.Message.(type) {
	case *MessageInitiateShutdown[K]:
		log.Debug("[GronosMessage] [InitiateShutdown]")
		defer messageInitiateShutdownPool.Put(m.Message)
		return g.handleInitiateShutdown(state, m.Metadata, msg.Data, msg.Result), true
	case *MessageInitiateContextCancellation[K]:
		log.Debug("[GronosMessage] [InitiateContextCancellation]")
		defer messageInitiateContextCancellationPool.Put(m.Message)
		return g.handleInitiateContextCancellation(state, m.Metadata, msg.Data, msg.Result), true
	}
	return nil, false
}

func (g *gronos[K]) handleInitiateShutdown(state *gronosState[K], metadata *Metadata[K], data Void, future Future[Void]) error {
	if state.shutting.Load() {
		log.Error("[GronosMessage] already shutting down handleInitiateShutdown")
		return nil
	}
	log.Debug("[GronosMessage] Initiating shutdown")
	state.shutting.Store(true)
	return g.initiateShutdownProcess(state, ShutdownKindTerminate)
}

func (g *gronos[K]) handleInitiateContextCancellation(state *gronosState[K], metadata *Metadata[K], data Void, future Future[Void]) error {
	if state.shutting.Load() {
		log.Error("[GronosMessage] already shutting down handleInitiateContextCancellation")
		return nil
	}
	log.Debug("[GronosMessage] Initiating context cancellation shutd")
	state.shutting.Store(true)
	return g.initiateShutdownProcess(state, ShutdownKindCancel)
}

func (g *gronos[K]) initiateShutdownProcess(state *gronosState[K], kind ShutdownKind) error {
	log.Debug("[GronosMessage] Initiating shutdown process", kind)

	localKeys := g.getLocalKeys(state)

	g.triggerShutdownForApps(state, localKeys, kind)

	var endStates atomic.Bool
	endStates.Store(false)

	// state watcher
	go func() {
		for !endStates.Load() {
			allEqual := true
			state.mstatus.Range(func(_ K, value StatusState) bool {
				if stateNumber(value) != 3 {
					allEqual = false
					return false // stops here
				}
				return true
			})
			if allEqual {
				endStates.Store(true)
			}
			runtime.Gosched()
		}
	}()

	whenAll := make(chan struct{})

	// watcher of the state
	go func() {
		for !endStates.Load() {
			runtime.Gosched()
		}
		close(whenAll)
	}()

	// Now that we triggered the shutdown for all the apps, we need to monitor the situation
	go func() {

		if g.config.immediatePeriod > 0 {
			select {
			case <-whenAll:
				log.Debug("[GronosMessage] all app really shutdown")
			// you're really dead fr fr no cap
			case <-time.After(g.config.gracePeriod + g.config.immediatePeriod):
				switch value := g.getSystemMetadata().(type) {
				case Success[*Metadata[K]]:
					switch g.enqueue(ChannelTypePublic, value.Value, NewMessageGracePeriodExceeded[K]()).Get().(type) {
					case Success[Void]:
						log.Debug("[GronosMessage] sent grace period exceeded")
					case Failure:
						log.Error("[GronosMessage] failed to send grace period exceeded")
					}
				case Failure:
					log.Error("[GronosMessage] failed to get system metadata for grace period exceeded")
				}
			}
		}

		switch value := g.getSystemMetadata().(type) {
		case Success[*Metadata[K]]:
			switch g.enqueue(ChannelTypePublic, value.Value, NewMessageShutdownComplete[K]()).Get().(type) {
			case Success[Void]:
				log.Debug("[GronosMessage] sent shutdown complete")
			case Failure:
				log.Error("[GronosMessage] failed to send shutdown complete")
			}
		case Failure:
			log.Error("[GronosMessage] failed to get system metadata for shutdown complete")
		}

		log.Debug("[GronosMessage] sent shutdown complete")
	}()

	return nil
}

func (g *gronos[K]) getLocalKeys(state *gronosState[K]) []K {
	localKeys := make([]K, 0)
	state.mkeys.Range(func(key, value K) bool {
		localKeys = append(localKeys, key)
		return true
	})
	return localKeys
}

func (g *gronos[K]) triggerShutdownForApps(state *gronosState[K], localKeys []K, kind ShutdownKind) {
	for _, key := range localKeys {
		if alive, ok := state.mali.Load(key); ok && alive {
			g.sendShutdownMessage(key, kind)
		}
	}
}

func (g *gronos[K]) sendShutdownMessage(key K, kind ShutdownKind) {
	switch value := g.getSystemMetadata().(type) {
	case Success[*Metadata[K]]:
		if kind == ShutdownKindTerminate {
			log.Debug("[GronosMessage] sent forced shutdown process terminate", key)
			switch g.enqueue(ChannelTypePublic, value.Value, NewMessageForceTerminateShutdown[K](key)).Get().(type) {
			case Success[Void]:
				log.Debug("[GronosMessage] sent forced shutdown process terminate", key)
			case Failure:
				log.Error("[GronosMessage] failed to send forced shutdown process terminate", key)
				// what's the backup?
			}
		} else {
			log.Debug("[GronosMessage] sent forced shutdown process cancel", key)
			switch g.enqueue(ChannelTypePublic, value.Value, NewMessageForceCancelShutdown[K](key, nil)).Get().(type) {
			case Success[Void]:
				log.Debug("[GronosMessage] sent forced shutdown process cancel", key)
			case Failure:
				log.Error("[GronosMessage] failed to send forced shutdown process cancel", key)
				// what's the backup?
			}
		}
	case Failure:
		log.Error("[GronosMessage] failed to get system metadata")
	}
}
