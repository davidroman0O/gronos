package gronos

import (
	"fmt"
	"sync"
	"time"

	"github.com/charmbracelet/log"
)

type MessageGracePeriodExceeded[K Primitive] struct {
	FutureMessage[Void, Void]
}

var messageGracePeriodExceededPoolInited bool = false
var messageGracePeriodExceededPool sync.Pool

func NewMessageGracePeriodExceeded[K Primitive]() *MessageGracePeriodExceeded[K] {
	if !messageGracePeriodExceededPoolInited {
		messageGracePeriodExceededPoolInited = true
		messageGracePeriodExceededPool = sync.Pool{
			New: func() any {
				return &MessageGracePeriodExceeded[K]{}
			},
		}
	}
	return &MessageGracePeriodExceeded[K]{
		FutureMessage: FutureMessage[Void, Void]{
			Data:   Void{},
			Result: NewFuture[Void](),
		},
	}
}

type MessageShutdownComplete[K Primitive] struct {
	FutureMessage[Void, Void]
}

var messageShutdownCompletePoolInited bool = false
var messageShutdownCompletePool sync.Pool

func NewMessageShutdownComplete[K Primitive]() *MessageShutdownComplete[K] {
	if !messageShutdownCompletePoolInited {
		messageShutdownCompletePoolInited = true
		messageShutdownCompletePool = sync.Pool{
			New: func() any {
				return &MessageShutdownComplete[K]{}
			},
		}
	}
	return &MessageShutdownComplete[K]{
		FutureMessage: FutureMessage[Void, Void]{
			Data:   Void{},
			Result: NewFuture[Void](),
		},
	}
}

type MessageCheckAutomaticShutdown[K Primitive] struct {
	FutureMessage[Void, Void]
}

var messageCheckAutomaticShutdownPoolInited bool = false
var messageCheckAutomaticShutdownPool sync.Pool

func NewMessageCheckAutomaticShutdown[K Primitive]() *MessageCheckAutomaticShutdown[K] {
	if !messageCheckAutomaticShutdownPoolInited {
		messageCheckAutomaticShutdownPoolInited = true
		messageCheckAutomaticShutdownPool = sync.Pool{
			New: func() any {
				return &MessageCheckAutomaticShutdown[K]{}
			},
		}
	}
	return &MessageCheckAutomaticShutdown[K]{
		FutureMessage: FutureMessage[Void, Void]{
			Data:   Void{},
			Result: NewFuture[Void](),
		},
	}
}

type MessageInitiateShutdown[K Primitive] struct {
	FutureMessage[Void, Void]
}

var messageInitiateShutdownPoolInited bool = false
var messageInitiateShutdownPool sync.Pool

func NewMessageInitiateShutdown[K Primitive]() *MessageInitiateShutdown[K] {
	if !messageInitiateShutdownPoolInited {
		messageInitiateShutdownPoolInited = true
		messageInitiateShutdownPool = sync.Pool{
			New: func() any {
				return &MessageInitiateShutdown[K]{}
			},
		}
	}
	return &MessageInitiateShutdown[K]{
		FutureMessage: FutureMessage[Void, Void]{
			Data:   Void{},
			Result: NewFuture[Void](),
		},
	}
}

type MessageDestroy[K Primitive] struct {
	FutureMessage[Void, Void]
}

var messageDestroyPoolInited bool = false
var messageDestroyPool sync.Pool

func NewMessageDestroy[K Primitive]() *MessageDestroy[K] {
	if !messageDestroyPoolInited {
		messageDestroyPoolInited = true
		messageDestroyPool = sync.Pool{
			New: func() any {
				return &MessageDestroy[K]{}
			},
		}
	}
	return &MessageDestroy[K]{
		FutureMessage: FutureMessage[Void, Void]{
			Data:   Void{},
			Result: NewFuture[Void](),
		},
	}
}

func (g *gronos[K]) handleShutdownStagesMessage(state *gronosState[K], m *MessagePayload[K]) (error, bool) {
	switch msg := m.Message.(type) {
	case *MessageShutdownProgress[K]:
		log.Debug("[GronosMessage] [ShutdownProgress]")
		defer messageShutdownProgressPool.Put(msg)
		return g.handleShutdownProgress(state, m.Metadata, msg.Data, msg.Result), true
	case *MessageShutdownComplete[K]:
		log.Debug("[GronosMessage] [ShutdownComplete]")
		defer messageShutdownCompletePool.Put(msg)
		return g.handleShutdownComplete(state, m.Metadata, msg.Data, msg.Result), true
	case *MessageCheckAutomaticShutdown[K]:
		log.Debug("[GronosMessage] [CheckAutomaticShutdown]")
		defer messageCheckAutomaticShutdownPool.Put(msg)
		return g.handleCheckAutomaticShutdown(state, m.Metadata, msg.Data, msg.Result), true
	case *MessageDestroy[K]:
		log.Debug("[GronosMessage] [Destroy]")
		defer messageDestroyPool.Put(msg)
		return g.handleDestroy(state, m.Metadata, msg.Data, msg.Result), true
	case *MessageGracePeriodExceeded[K]:
		log.Debug("[GronosMessage] [GracePeriodExceeded]")
		defer messageGracePeriodExceededPool.Put(msg)
		return g.handleGracePeriodExceeded(state, m.Metadata, msg.Data, msg.Result), true
	}
	return nil, false
}

// You choose violence
func (g *gronos[K]) handleGracePeriodExceeded(state *gronosState[K], metadata *Metadata[K], data Void, future Future[Void]) error {
	if !state.allApplicationsTerminated() {
		log.Error("[Gronos] Shutdown grace period exceeded, some applications failed to terminate in a timely manner")
		panic("grace period exceeded")
	}
	return nil
}

func (g *gronos[K]) handleShutdownProgress(state *gronosState[K], metadata *Metadata[K], data struct{ RemainingApps int }, future Future[Void]) error {
	log.Debug("[GronosMessage] Shutdown progress", "remaining", data.RemainingApps)

	switch value := g.getSystemMetadata().(type) {
	case Success[*Metadata[K]]:
		if data.RemainingApps == 0 {
			g.enqueue(ChannelTypePublic, value.Value, NewMessageShutdownComplete[K](), WithDefault())
		} else {
			// Check again after a short delay
			time.AfterFunc(time.Second, func() {
				g.checkRemainingApps(state, value.Value)
			})
		}
		future.Close()
	case Failure:
		future.PublishError(fmt.Errorf("failed to get system metadata"))
	}

	return nil
}

func (g *gronos[K]) checkRemainingApps(state *gronosState[K], metadata *Metadata[K]) {
	var remainingApps int
	state.mkeys.Range(func(key, value K) bool {
		if alive, ok := state.mali.Load(key); ok && alive {
			remainingApps++
		}
		return true
	})
	g.enqueue(ChannelTypePublic, metadata, NewMessageShutdownProgress[K](remainingApps), WithDefault())
}

func (g *gronos[K]) handleShutdownComplete(state *gronosState[K], metadata *Metadata[K], data Void, future Future[Void]) error {
	log.Debug("[GronosMessage] Shutdown waiting")
	go func() {
		if g.config.gracePeriod > 0 {
			<-time.After(g.config.gracePeriod)
		}
		if g.config.wait {
			state.wait.Wait()
		}
		switch value := g.getSystemMetadata().(type) {
		case Success[*Metadata[K]]:
			g.enqueue(ChannelTypePublic, value.Value, NewMessageDestroy[K](), WithDefault())
		case Failure:
			future.PublishError(fmt.Errorf("failed to get system metadata"))
			return
		}
		future.Close()
		log.Debug("[GronosMessage] Shutdown complete")
	}()
	state.wait.Wait() // TODO: not sure anymore that we need it
	return nil
}

func (g *gronos[K]) handleCheckAutomaticShutdown(state *gronosState[K], metadata *Metadata[K], data Void, future Future[Void]) error {
	log.Debug("[GronosMessage] Checking automatic shutdown")

	// we already detected an automatic shutdown
	if state.automaticShutdown.Load() {
		future.Close()
		return nil
	}

	allDead := true
	state.mali.Range(func(_ K, value bool) bool {
		if value {
			allDead = false
			return false // stop iteration
		}
		return true
	})

	if allDead {
		log.Debug("[GronosMessage] All applications are dead, initiating shutdown")
		state.automaticShutdown.Store(true)
		switch value := g.getSystemMetadata().(type) {
		case Success[*Metadata[K]]:
			// asynchronously trigger it
			g.enqueue(ChannelTypePublic, value.Value, NewMessageInitiateShutdown[K](), WithDefault())
		case Failure:
			future.PublishError(fmt.Errorf("failed to get system metadata"))
			return nil
		}
	}

	future.Close()
	return nil
}

func (g *gronos[K]) handleDestroy(state *gronosState[K], metadata *Metadata[K], data Void, future Future[Void]) error {
	log.Debug("[GronosMessage] Destroying gronos")
	g.comClosed.Store(true)
	log.Debug("[GronosMessage] run closing")
	close(g.publicChn)
	close(g.privateChn)
	close(g.doneChan)
	log.Debug("[GronosMessage] run closed - no more channels")
	return nil
}
