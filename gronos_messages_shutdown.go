package gronos

import (
	"time"

	"github.com/charmbracelet/log"
)

type InitiateShutdown[K comparable] struct{}
type InitiateContextCancellation[K comparable] struct{}
type ShutdownProgress[K comparable] struct {
	RemainingApps int
}

func (g *gronos[K]) handleShutdownMessage(state *gronosState[K], m Message) (error, bool) {
	switch m.(type) {
	case *InitiateShutdown[K]:
		log.Debug("[GronosMessage] [InitiateShutdown]")
		return g.handleInitiateShutdown(state), true
	case *InitiateContextCancellation[K]:
		log.Debug("[GronosMessage] [InitiateContextCancellation]")
		return g.handleInitiateContextCancellation(state), true
	}
	return nil, false
}

func (g *gronos[K]) handleInitiateShutdown(state *gronosState[K]) error {
	if state.shutting.Load() {
		log.Error("[GronosMessage] already shutting down handleInitiateShutdown")
		return nil
	}
	log.Debug("[GronosMessage] Initiating shutdown")
	state.shutting.Store(true)
	return g.initiateShutdownProcess(state, ShutdownKindTerminate)
}

func (g *gronos[K]) handleInitiateContextCancellation(state *gronosState[K]) error {
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
	dones := g.triggerShutdownForApps(state, localKeys, kind)

	time.AfterFunc(g.config.gracePeriod, func() {
		g.sendMessage(MsgGracePeriodExceeded[K]())
	})

	g.watchForShutdownCompletion(dones)

	return nil
}

func (g *gronos[K]) getLocalKeys(state *gronosState[K]) []K {
	localKeys := make([]K, 0)
	state.mkeys.Range(func(key, value interface{}) bool {
		localKeys = append(localKeys, key.(K))
		return true
	})
	return localKeys
}

func (g *gronos[K]) triggerShutdownForApps(state *gronosState[K], localKeys []K, kind ShutdownKind) []chan struct{} {
	dones := []chan struct{}{}
	for _, key := range localKeys {
		value, ok := state.mdone.Load(key)
		if !ok {
			log.Debug("[GronosMessage] no done channel", key)
			continue
		}
		dones = append(dones, value.(chan struct{}))

		if alive, ok := state.mali.Load(key); ok && alive.(bool) {
			g.sendShutdownMessage(key, kind)
		}
	}
	return dones
}

func (g *gronos[K]) sendShutdownMessage(key K, kind ShutdownKind) {
	if kind == ShutdownKindTerminate {
		log.Debug("[GronosMessage] sent forced shutdown process terminate", key)
		g.sendMessage(MsgForceTerminateShutdown(key))
	} else {
		log.Debug("[GronosMessage] sent forced shutdown process cancel", key)
		g.sendMessage(MsgForceCancelShutdown(key, nil))
	}
}

func (g *gronos[K]) watchForShutdownCompletion(dones []chan struct{}) {
	go func() {
		log.Debug("[GronosMessage] waiting for all applications to be shutdown")
		for _, done := range dones {
			<-done
		}
		log.Debug("[GronosMessage] all applications are shutdown")
		g.com <- &ShutdownComplete[K]{}
		log.Debug("[GronosMessage] sent shutdown complete")
	}()
}
