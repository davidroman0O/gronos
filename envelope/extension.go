package envelope

import (
	"context"
	"reflect"
	"sync"

	"github.com/davidroman0O/gronos"
)

type EvelopeExtension[K comparable] struct {
	registry      sync.Map
	registryPools sync.Map
}

func New[K comparable]() *EvelopeExtension[K] {
	return &EvelopeExtension[K]{}
}

func (w *EvelopeExtension[K]) OnStart(ctx context.Context, errChan chan<- error) error {
	return nil
}

func (w *EvelopeExtension[K]) OnNewRuntime(ctx context.Context) context.Context {
	return ctx
}

func (w *EvelopeExtension[K]) OnStopRuntime(ctx context.Context) context.Context {
	return ctx
}

func (w *EvelopeExtension[K]) OnStop(ctx context.Context, errChan chan<- error) error {
	return nil
}

func (w *EvelopeExtension[K]) closeAllComponents(errChan chan<- error) error {
	return nil
}

type AddKind[K comparable] struct {
	fn registeration
	gronos.RequestMessage[K, bool]
}

func MsgAdd[K comparable](fn registeration) (<-chan bool, *AddKind[K]) {
	msg := &AddKind[K]{
		fn: fn,
	}
	msg.Response = make(chan bool)
	return msg.Response, msg
}

func (w *EvelopeExtension[K]) OnMsg(ctx context.Context, m *gronos.MessagePayload) error {
	switch msg := m.Message.(type) {
	case *AddKind[K]:
		defer close(msg.Response)
		w.register(msg.fn)
		msg.Response <- true
		return nil
	default:
		return gronos.ErrUnhandledMessage
	}
}

type registeration func() (string, reflect.Type)

func Register[T any]() registeration {
	return func() (string, reflect.Type) {
		// signature, type
		return reflect.TypeFor[T]().Name(), reflect.TypeFor[T]()
	}
}

func (r *EvelopeExtension[K]) register(registeration registeration) {
	eventSignature, event := registeration()
	r.registry.Store(eventSignature, event)
	r.registryPools.Store(eventSignature, sync.Pool{
		New: func() interface{} {
			return reflect.New(event).Interface()
		},
	})
}

func signatureFor[T any]() string {
	return reflect.TypeFor[T]().Name()
}

func (r *EvelopeExtension[K]) TranslateMessageToEvent(metadata map[string]string, payload []byte) (*Envelope, error) {
	// var evt *Event
	// var err error

	// if evt, err = ParseEvent(metadata, payload); err != nil {
	// 	return nil, err
	// }

	// if r.eventTypeByName[evt.EventName] == nil {
	// 	return nil, fmt.Errorf("event type is not registered")
	// }

	// // We do not need to really assign a type to the Payload
	// // In Go, we rather have a function with the signature func(something interface{}) so we can do the triage ourselves.
	// // Also, we might want to do a `switch value := something.(type)` which would help the readability.
	// if valueType, ok := r.eventTypeByName[evt.EventName]; ok {
	// 	empty := reflect.New(valueType).Interface()
	// 	if err := json.Unmarshal(payload, &empty); err != nil {
	// 		return nil, err
	// 	}
	// 	evt.Payload = reflect.ValueOf(empty).Elem().Interface()
	// }

	// evt.Metadata = metadata

	return nil, nil
}
