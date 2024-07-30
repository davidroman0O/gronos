package gronos

import (
	"errors"
	"fmt"
)

type HeaderMessage[K comparable] struct {
	Key K
}

// deadLetterMessage represents a message indicating that an application has terminated with an error.
type deadLetterMessage[K comparable] struct {
	HeaderMessage[K]
	Reason error
}

func MsgDeadLetter[K comparable](k K, reason error) deadLetterMessage[K] {
	return deadLetterMessage[K]{HeaderMessage: HeaderMessage[K]{Key: k}, Reason: reason}
}

// terminatedMessage represents a message indicating that an application has terminated normally.
type terminatedMessage[K comparable] struct {
	HeaderMessage[K]
}

func MsgTerminated[K comparable](k K) terminatedMessage[K] {
	return terminatedMessage[K]{HeaderMessage: HeaderMessage[K]{Key: k}}
}

// contextTerminatedMessage represents a message indicating that an application's context has been terminated.
type contextTerminatedMessage[K comparable] struct {
	HeaderMessage[K]
	Err error
}

func MsgContextTerminated[K comparable](k K, err error) contextTerminatedMessage[K] {
	return contextTerminatedMessage[K]{HeaderMessage: HeaderMessage[K]{Key: k}, Err: err}
}

type errorMessage[K comparable] struct {
	HeaderMessage[K]
	Err error
}

func MsgError[K comparable](k K, err error) errorMessage[K] {
	return errorMessage[K]{HeaderMessage: HeaderMessage[K]{Key: k}, Err: err}
}

// addMessage represents a message to add a new application to the gronos system.
type addMessage[K comparable] struct {
	HeaderMessage[K]
	App RuntimeApplication
}

func MsgAdd[K comparable](k K, app RuntimeApplication) addMessage[K] {
	return addMessage[K]{HeaderMessage: HeaderMessage[K]{Key: k}, App: app}
}

type contextCancelledMessage[K comparable] struct{}

func msgContextCancelled[K comparable]() contextCancelledMessage[K] {
	return contextCancelledMessage[K]{}
}

// ShutdownMessage represents a message to initiate shutdown
type ShutdownMessage struct{}

// ErrUnhandledMessage is returned when a message type is not recognized.
var ErrUnhandledMessage = errors.New("unhandled message type")

func (g *gronos[K]) handleGronosMessage(m Message) error {
	switch msg := m.(type) {
	case deadLetterMessage[K]:
		return g.handleDeadLetter(msg)
	case terminatedMessage[K]:
		return g.handleTerminated(msg)
	case contextTerminatedMessage[K]:
		return g.handleContextTerminated(msg)
	case addMessage[K]:
		return g.Add(msg.Key, msg.App)
	case errorMessage[K]:
		return errors.Join(fmt.Errorf("app error %v", msg.Key), msg.Err)
	case StatusMessage[K]:
		g.handleStatusMessage(msg)
		return nil
	case ShutdownMessage:
		g.initiateShutdown(false)
		return nil
	case contextCancelledMessage[K]:
		g.initiateShutdown(true)
		return nil
	default:
		return ErrUnhandledMessage
	}
}

func (g *gronos[K]) handleDeadLetter(msg deadLetterMessage[K]) error {
	value, ok := g.applications.Load(msg.Key)
	if !ok {
		return nil
	}
	app, ok := value.(applicationContext[K])
	if !ok {
		return nil
	}
	if !app.alive.Load() {
		return nil
	}
	app.alive.Store(false)
	app.reason = msg.Reason
	app.closer()
	g.applications.Store(app.k, app)
	if msg.Reason != nil && msg.Reason.Error() != "shutdown" {
		return errors.Join(fmt.Errorf("app error %v", msg.Key), msg.Reason)
	}
	return nil
}

func (g *gronos[K]) handleTerminated(msg terminatedMessage[K]) error {
	value, ok := g.applications.Load(msg.Key)
	if !ok {
		return nil
	}
	app, ok := value.(applicationContext[K])
	if !ok {
		return nil
	}
	if !app.alive.Load() {
		return nil
	}
	app.alive.Store(false)
	app.closer()
	g.applications.Store(app.k, app)
	return nil
}

func (g *gronos[K]) handleContextTerminated(msg contextTerminatedMessage[K]) error {
	value, ok := g.applications.Load(msg.Key)
	if !ok {
		return nil
	}
	app, ok := value.(applicationContext[K])
	if !ok {
		return nil
	}
	if !app.alive.Load() {
		return nil
	}
	app.alive.Store(false)
	app.cancel()
	g.applications.Store(app.k, app)
	return nil
}

func (g *gronos[K]) handleStatusMessage(msg StatusMessage[K]) {
	g.statuses.Store(msg.Key, msg.State)
}
