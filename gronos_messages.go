package gronos

import (
	"errors"
	"fmt"
	"reflect"

	"github.com/charmbracelet/log"
)

/// Each message is a set of properties that will mutate one by one the state of the system

// Message is an interface type for internal communication within gronos.
type Message interface{}

type Envelope[K comparable] struct {
	From K
	Message
}

// Composable header
type KeyMessage[K comparable] struct {
	Key K
}

// Used for generic requests
type RequestMessage[K comparable, Y any] struct {
	KeyMessage[K]
	Response chan Y
}

// handleMessage processes incoming messages and updates the gronos state accordingly.
func (g *gronos[K]) handleMessage(state *gronosState[K], m *MessagePayload) error {

	log.Debug("[GronosMessage] handle message", "name", m.Metadata["name"], "metadata", m.Metadata, "message", m.Message)

	// clean up pool data
	defer func() {
		metadataPool.Put(m.Metadata)
		messagePayloadPool.Put(m)
	}()

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

func (g *gronos[K]) handleGronosMessage(state *gronosState[K], m *MessagePayload) error {
	log.Debug("[GronosMessage] handle gronos message", reflect.TypeOf(m).Name(), m)

	// Error should always be the highest priority
	switch msg := m.Message.(type) {
	case *RuntimeError[K]:
		log.Debug("[GronosMessage] [RuntimeError]")
		g.errChan <- msg.Error
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
	state.mali.Range(func(_, value interface{}) bool {
		if value.(bool) {
			allTerminated = false
			return false // stop iteration
		}
		return true
	})
	return allTerminated
}
