package gronos

import (
	"sync"
	"time"

	"github.com/charmbracelet/log"
)

// TODO: sync.Pool for all messages
type GracePeriodExceededMessage[K Primitive] struct {
	KeyMessage[K]
}

var gracePeriodExceededPool = sync.Pool{
	New: func() interface{} {
		return &GracePeriodExceededMessage[string]{}
	},
}

func MsgGracePeriodExceeded[K Primitive]() *GracePeriodExceededMessage[K] {
	return gracePeriodExceededPool.Get().(*GracePeriodExceededMessage[K])
}

type ShutdownComplete[K Primitive] struct{}

type CheckAutomaticShutdown[K Primitive] struct {
	RequestMessage[K, struct{}]
}

func MsgCheckAutomaticShutdown[K Primitive]() (chan struct{}, *CheckAutomaticShutdown[K]) {
	done := make(chan struct{})
	msg := &CheckAutomaticShutdown[K]{}
	msg.Response = done
	return done, msg
}

// Message creation functions
func MsgInitiateShutdown[K Primitive]() *InitiateShutdown[K] {
	return &InitiateShutdown[K]{}
}

type Destroy[K Primitive] struct{}

func MsgDestroy[K Primitive]() *Destroy[K] {
	return &Destroy[K]{}
}

func (g *gronos[K]) handleShutdownStagesMessage(state *gronosState[K], m *MessagePayload[K]) (error, bool) {
	switch msg := m.Message.(type) {
	case *ShutdownProgress[K]:
		log.Debug("[GronosMessage] [ShutdownProgress]")
		return g.handleShutdownProgress(state, msg.RemainingApps), true
	case *ShutdownComplete[K]:
		log.Debug("[GronosMessage] [ShutdownComplete]")
		return g.handleShutdownComplete(state), true
	case *CheckAutomaticShutdown[K]:
		log.Debug("[GronosMessage] [CheckAutomaticShutdown]")
		return g.handleCheckAutomaticShutdown(state, msg.Response), true
	case *Destroy[K]:
		log.Debug("[GronosMessage] [Destroy]")
		return g.handleDestroy(state), true
	case *GracePeriodExceededMessage[K]:
		log.Debug("[GronosMessage] [GracePeriodExceeded]")
		defer gracePeriodExceededPool.Put(msg)
		return g.handleGracePeriodExceeded(state), true
	}
	return nil, false
}

// You choose violence
func (g *gronos[K]) handleGracePeriodExceeded(state *gronosState[K]) error {
	if !state.allApplicationsTerminated() {
		log.Error("[Gronos] Shutdown grace period exceeded, some applications failed to terminate in a timely manner")
		panic("grace period exceeded")
	}
	return nil
}

func (g *gronos[K]) handleShutdownProgress(state *gronosState[K], remainingApps int) error {
	log.Debug("[GronosMessage] Shutdown progress", "remaining", remainingApps)
	metadata := g.getSystemMetadata()
	if remainingApps == 0 {
		g.sendMessage(metadata, &ShutdownComplete[K]{})
	} else {
		// Check again after a short delay
		time.AfterFunc(time.Second, func() {
			g.checkRemainingApps(state)
		})
	}
	return nil
}

func (g *gronos[K]) checkRemainingApps(state *gronosState[K]) {
	var remainingApps int
	state.mkeys.Range(func(key, value K) bool {
		if alive, ok := state.mali.Load(key); ok && alive {
			remainingApps++
		}
		return true
	})
	metadata := g.getSystemMetadata()
	g.sendMessage(metadata, &ShutdownProgress[K]{RemainingApps: remainingApps})
}

func (g *gronos[K]) handleShutdownComplete(state *gronosState[K]) error {
	log.Debug("[GronosMessage] Shutdown waiting")
	go func() {
		if g.config.gracePeriod > 0 {
			<-time.After(g.config.gracePeriod)
		}
		if g.config.wait {
			state.wait.Wait()
		}
		metadata := g.getSystemMetadata()
		if !g.sendMessage(metadata, &Destroy[K]{}) {
			log.Error("[GronosMessage] Failed to send destroy message")
		}
		log.Debug("[GronosMessage] Shutdown complete")
	}()
	state.wait.Wait()
	return nil
}

func (g *gronos[K]) handleCheckAutomaticShutdown(state *gronosState[K], response chan struct{}) error {
	log.Debug("[GronosMessage] Checking automatic shutdown")

	// we already detected an automatic shutdown
	if state.automaticShutdown.Load() {
		close(response)
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
		metadata := g.getSystemMetadata()
		g.sendMessage(metadata, MsgInitiateShutdown[K]()) // asynchronously trigger it
	}

	close(response)
	return nil
}

func (g *gronos[K]) handleDestroy(state *gronosState[K]) error {
	log.Debug("[GronosMessage] Destroying gronos")
	g.comClosed.Store(true)
	log.Debug("[GronosMessage] run closing")
	close(g.publicChn)
	close(g.privateChn)
	close(g.doneChan)
	log.Debug("[GronosMessage] run closed - no more channels")
	return nil
}
