package gronos

import (
	"errors"
	"fmt"
	"sync"

	"github.com/charmbracelet/log"
)

/// Each message is a set of properties that will mutate one by one the state of the system

type MessageProcessor[K Primitive, D any, F any] func(state *gronosState[K], metadata *Metadata[K], data D, future Future[F]) (error, bool)

type MessagePoolProcessor[K Primitive, D any, F any] struct {
	pool      sync.Pool
	processor MessageProcessor[K, D, F]
}

// Message is given by the user or the system, represent the data of a passed message.
type Message interface{}

type Envelope[K Primitive] struct {
	From K
	Message
}

// Composable header
type KeyMessage[K Primitive] struct {
	Key K
}

// TODO: remove that after the FutureMessage is used everywhere
// Used for generic requests
type RequestMessage[K Primitive, Y any] struct {
	KeyMessage[K]
	Response chan Y
}

type MessagePayload[K Primitive] struct {
	*Metadata[K]
	Message
}

// handleMessage processes incoming messages and updates the gronos state accordingly.
func (g *gronos[K]) handleMessage(state *gronosState[K], m *MessagePayload[K]) error {

	log.Debug("[GronosMessage] handle message", "metadata", m.String(), "message", m.Message)

	// Try to handle the message with the gronos core
	coreErr := g.handleGronosMessage(state, m)

	// If the gronos core couldn't handle it or returned an error, pass it to extensions
	if coreErr != nil {
		if errors.Is(coreErr, ErrUnhandledMessage) {
			for _, ext := range g.extensions {
				extErr := ext.OnMsg(g.ctx, m)
				if extErr == nil {
					// Message was handled by an extension
					return nil
				}
				if errors.Is(extErr, ErrUnmanageExtensionMessage) {
					// Collect extension errors, but continue trying other extensions
					coreErr = errors.Join(coreErr, extErr)
				}
			}
		}
	}

	// If the message wasn't handled by core or any extension, return an error
	if errors.Is(coreErr, ErrUnhandledMessage) {
		return fmt.Errorf("unhandled message type: %T", m)
	}

	// Return any errors encountered during message handling
	return coreErr
}

func (g *gronos[K]) handleGronosMessage(state *gronosState[K], m *MessagePayload[K]) error {

	// While read the rest of the functions: if you're asking "wouldn't be simpler with a map?"
	// the answer is simple, i don't want to impede on the performance of the user of gronos
	// since the compiler will optmize for me the switch statement under those functions
	// If i use a map, sure it might add some flexibility FOR ME but who cares? I will manage it
	// i don't want YOU to have a map that will add extract code execution time to your code
	// If you have to use a library, it has to use a less cpu cycle as possible

	// Error should always be the highest priority
	switch msg := m.Message.(type) {
	case *MessageRuntimeError[K]:
		log.Debug("[GronosMessage] [RuntimeError]")
		g.errChan <- msg.Data.Error
		messageRuntimeErrorPool.Put(msg) // put it back in the pool
		return nil
	}

	// - add runtime application
	// - force cancel shutdown
	// - force terminate shutdown
	// - cancelled shutdown
	// - terminated shutdown
	// - panicked shutdown
	// - errored shutdown
	if err, handled := g.handleRuntimeApplicationMessage(state, m); handled {
		return err
	}

	// - request status
	// - request alive
	// - request reason
	// - request all alive
	// - request status async
	// - request status async
	if err, handled := g.handleStateMessage(state, m); handled {
		return err
	}

	// - initiate shutdown
	// - initiate context cancellation
	if err, handled := g.handleShutdownMessage(state, m); handled {
		return err
	}

	// - shutdown progress
	// - shutdown complete
	// - check automatic shutdown
	// - destroy
	// - grace period exceeded
	if err, handled := g.handleShutdownStagesMessage(state, m); handled {
		return err
	}

	return ErrUnhandledMessage
}

func (state *gronosState[K]) allApplicationsTerminated() bool {
	allTerminated := true
	state.mali.Range(func(_ K, value bool) bool {
		if value {
			allTerminated = false
			return false // stop iteration
		}
		return true
	})
	return allTerminated
}
