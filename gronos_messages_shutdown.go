package gronos

import (
	"runtime"
	"sync/atomic"
	"time"

	"github.com/charmbracelet/log"
)

type InitiateShutdown[K comparable] struct{}
type InitiateContextCancellation[K comparable] struct{}
type ShutdownProgress[K comparable] struct {
	RemainingApps int
}

func MsgInitiateContextCancellation[K comparable]() *InitiateContextCancellation[K] {
	return &InitiateContextCancellation[K]{}
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

	g.triggerShutdownForApps(state, localKeys, kind)

	var endStates atomic.Bool
	endStates.Store(false)

	// state watcher
	go func() {
		for !endStates.Load() {
			allEqual := true
			state.mstatus.Range(func(_, value interface{}) bool {
				if stateNumber(value.(StatusState)) != 3 {
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

		select {
		case <-whenAll:
			log.Debug("[GronosMessage] all app really shutdown")
		case <-time.AfterFunc(g.config.gracePeriod, func() {
			g.sendMessage(MsgGracePeriodExceeded[K]())
		}).C:
			log.Debug("[GronosMessage] grace period exceeded")
		}

		log.Debug("[GronosMessage] check all application are last state")

		log.Debug("[GronosMessage] all applications are shutdown")
		g.com <- &ShutdownComplete[K]{}
		log.Debug("[GronosMessage] sent shutdown complete")
	}()

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

func (g *gronos[K]) triggerShutdownForApps(state *gronosState[K], localKeys []K, kind ShutdownKind) {
	for _, key := range localKeys {
		if alive, ok := state.mali.Load(key); ok && alive.(bool) {
			g.sendShutdownMessage(key, kind)
		}
	}
}

func (g *gronos[K]) sendShutdownMessage(key K, kind ShutdownKind) {
	if kind == ShutdownKindTerminate {
		log.Debug("[GronosMessage] sent forced shutdown process terminate", key)
		_, msg := MsgForceTerminateShutdown(key)
		if !g.sendMessage(msg) {
			log.Error("[GronosMessage] failed to send forced shutdown process terminate", key)
		}
	} else {
		log.Debug("[GronosMessage] sent forced shutdown process cancel", key)
		_, msg := MsgForceCancelShutdown(key, nil)
		if !g.sendMessage(msg) {
			log.Error("[GronosMessage] failed to send forced shutdown process cancel", key)
		}
	}
}
