package watermillextension

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/ThreeDotsLabs/watermill"
	"github.com/ThreeDotsLabs/watermill/message"
	"github.com/davidroman0O/gronos"
)

type ctxWatermill string

var ctxWatermillKey ctxWatermill

type WatermillMiddleware[K comparable] struct {
	pubs    sync.Map
	subs    sync.Map
	routers sync.Map
	logger  watermill.LoggerAdapter
}

func NewWatermillMiddleware[K comparable](logger watermill.LoggerAdapter) *WatermillMiddleware[K] {
	if logger == nil {
		logger = watermill.NewStdLogger(false, false)
	}
	return &WatermillMiddleware[K]{
		logger: logger,
	}
}

// Message types
type AddPublisherMessage[K comparable] struct {
	gronos.KeyMessage[K]
	Publisher                          message.Publisher
	gronos.RequestMessage[K, struct{}] // when it is done
}

type AddSubscriberMessage[K comparable] struct {
	gronos.KeyMessage[K]
	Subscriber                         message.Subscriber
	gronos.RequestMessage[K, struct{}] // when it is done
}

type AddRouterMessage[K comparable] struct {
	gronos.KeyMessage[K]
	Router                             *message.Router
	gronos.RequestMessage[K, struct{}] // when it is done
}

type ClosePublisherMessage[K comparable] struct {
	gronos.KeyMessage[K]
	gronos.RequestMessage[K, struct{}] // when it is done
}

type CloseSubscriberMessage[K comparable] struct {
	gronos.KeyMessage[K]
	gronos.RequestMessage[K, struct{}] // when it is done
}

type AddHandlerMessage[K comparable] struct {
	gronos.KeyMessage[K]
	HandlerName                        string
	SubscribeTopic                     string
	PublishTopic                       string
	HandlerFunc                        message.HandlerFunc
	gronos.RequestMessage[K, struct{}] // when it is done
}

// Sync pools for message types
var addPublisherPoolInited bool
var addPublisherPool sync.Pool

var addSubscriberPoolInited bool
var addSubscriberPool sync.Pool

var addRouterPoolInited bool
var addRouterPool sync.Pool

var closePublisherPoolInited bool
var closePublisherPool sync.Pool

var closeSubscriberPoolInited bool
var closeSubscriberPool sync.Pool

var addHandlerPoolInited bool
var addHandlerPool sync.Pool

// Message creation functions
func MsgAddPublisher[K comparable](key K, publisher message.Publisher) (<-chan struct{}, *AddPublisherMessage[K]) {
	if !addPublisherPoolInited {
		addPublisherPoolInited = true
		addPublisherPool = sync.Pool{
			New: func() interface{} {
				return &AddPublisherMessage[K]{}
			},
		}
	}
	msg := addPublisherPool.Get().(*AddPublisherMessage[K])
	msg.Key = key
	msg.Publisher = publisher
	msg.Response = make(chan struct{}, 1)
	return msg.Response, msg
}

func MsgAddSubscriber[K comparable](key K, subscriber message.Subscriber) (<-chan struct{}, *AddSubscriberMessage[K]) {
	if !addSubscriberPoolInited {
		addSubscriberPoolInited = true
		addSubscriberPool = sync.Pool{
			New: func() interface{} {
				return &AddSubscriberMessage[K]{}
			},
		}
	}
	msg := addSubscriberPool.Get().(*AddSubscriberMessage[K])
	msg.Key = key
	msg.Subscriber = subscriber
	msg.Response = make(chan struct{}, 1)
	return msg.Response, msg
}

func MsgAddRouter[K comparable](key K, router *message.Router) (<-chan struct{}, *AddRouterMessage[K]) {
	if !addRouterPoolInited {
		addRouterPoolInited = true
		addRouterPool = sync.Pool{
			New: func() interface{} {
				return &AddRouterMessage[K]{}
			},
		}
	}
	msg := addRouterPool.Get().(*AddRouterMessage[K])
	msg.Key = key
	msg.Router = router
	msg.Response = make(chan struct{}, 1)
	return msg.Response, msg
}

func MsgClosePublisher[K comparable](key K) (<-chan struct{}, *ClosePublisherMessage[K]) {
	if !closePublisherPoolInited {
		closePublisherPoolInited = true
		closePublisherPool = sync.Pool{
			New: func() interface{} {
				return &ClosePublisherMessage[K]{}
			},
		}
	}
	msg := closePublisherPool.Get().(*ClosePublisherMessage[K])
	msg.Key = key
	msg.Response = make(chan struct{}, 1)
	return msg.Response, msg
}

func MsgCloseSubscriber[K comparable](key K) (<-chan struct{}, *CloseSubscriberMessage[K]) {
	if !closeSubscriberPoolInited {
		closeSubscriberPoolInited = true
		closeSubscriberPool = sync.Pool{
			New: func() interface{} {
				return &CloseSubscriberMessage[K]{}
			},
		}
	}
	msg := closeSubscriberPool.Get().(*CloseSubscriberMessage[K])
	msg.Key = key
	msg.Response = make(chan struct{}, 1)
	return msg.Response, msg
}

func MsgAddHandler[K comparable](key K, handlerName, subscribeTopic, publishTopic string, handlerFunc message.HandlerFunc) (<-chan struct{}, *AddHandlerMessage[K]) {
	if !addHandlerPoolInited {
		addHandlerPoolInited = true
		addHandlerPool = sync.Pool{
			New: func() interface{} {
				return &AddHandlerMessage[K]{}
			},
		}
	}
	msg := addHandlerPool.Get().(*AddHandlerMessage[K])
	msg.Key = key
	msg.HandlerName = handlerName
	msg.SubscribeTopic = subscribeTopic
	msg.PublishTopic = publishTopic
	msg.HandlerFunc = handlerFunc
	msg.Response = make(chan struct{}, 1)
	return msg.Response, msg
}

// Extension methods
func (w *WatermillMiddleware[K]) OnStart(ctx context.Context, errChan chan<- error) error {
	w.logger.Debug("Starting Watermill middleware", nil)
	return nil
}

func (w *WatermillMiddleware[K]) OnNewRuntime(ctx context.Context) context.Context {
	return context.WithValue(ctx, ctxWatermillKey, w)
}

func (w *WatermillMiddleware[K]) OnStopRuntime(ctx context.Context) context.Context {
	return ctx
}

func (w *WatermillMiddleware[K]) OnStop(ctx context.Context, errChan chan<- error) error {
	w.logger.Debug("Stopping Watermill middleware", nil)
	w.closeAllComponents(errChan)
	return nil
}

func (w *WatermillMiddleware[K]) closeAllComponents(errChan chan<- error) error {
	var wg sync.WaitGroup
	errCh := make(chan error, 3) // For publishers, subscribers, and routers

	wg.Add(1)
	go func() {
		defer wg.Done()
		w.pubs.Range(func(key, value interface{}) bool {
			if err := value.(message.Publisher).Close(); err != nil {
				errCh <- fmt.Errorf("error closing publisher %v: %w", key, err)
			}
			return true
		})
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		w.subs.Range(func(key, value interface{}) bool {
			if err := value.(message.Subscriber).Close(); err != nil {
				errCh <- fmt.Errorf("error closing subscriber %v: %w", key, err)
			}
			return true
		})
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		w.routers.Range(func(key, value interface{}) bool {
			if err := value.(*RouterStatus).Router.Close(); err != nil {
				errCh <- fmt.Errorf("error closing router %v: %w", key, err)
			}
			return true
		})
	}()

	wg.Wait()
	close(errCh)

	var errs error
	for err := range errCh {
		errs = errors.Join(errs, err)
	}

	return errs
}

func (w *WatermillMiddleware[K]) OnMsg(ctx context.Context, m gronos.Message) error {
	switch msg := m.(type) {
	case *AddPublisherMessage[K]:
		defer addPublisherPool.Put(msg)
		return w.handleAddPublisher(ctx, msg)
	case *AddSubscriberMessage[K]:
		defer addSubscriberPool.Put(msg)
		return w.handleAddSubscriber(ctx, msg)
	case *AddRouterMessage[K]:
		defer addRouterPool.Put(msg)
		return w.handleAddRouter(ctx, msg)
	case *AddHandlerMessage[K]:
		defer addHandlerPool.Put(msg)
		return w.handleAddHandler(ctx, msg)
	case *ClosePublisherMessage[K]:
		defer closePublisherPool.Put(msg)
		return w.handleClosePublisher(ctx, msg)
	case *CloseSubscriberMessage[K]:
		defer closeSubscriberPool.Put(msg)
		return w.handleCloseSubscriber(ctx, msg)
	default:
		return gronos.ErrUnmanageExtensionMessage
	}
}

func (w *WatermillMiddleware[K]) handleAddPublisher(ctx context.Context, msg *AddPublisherMessage[K]) error {
	defer close(msg.Response)
	w.pubs.Store(msg.Key, msg.Publisher)
	w.logger.Debug("Added publisher", watermill.LogFields{"key": msg.Key})
	return nil
}

func (w *WatermillMiddleware[K]) handleAddSubscriber(ctx context.Context, msg *AddSubscriberMessage[K]) error {
	defer close(msg.Response)
	w.subs.Store(msg.Key, msg.Subscriber)
	w.logger.Debug("Added subscriber", watermill.LogFields{"key": msg.Key})
	return nil
}

func (w *WatermillMiddleware[K]) handleClosePublisher(ctx context.Context, msg *ClosePublisherMessage[K]) error {
	defer close(msg.Response)
	pub, ok := w.pubs.LoadAndDelete(msg.Key)
	if !ok {
		return fmt.Errorf("publisher not found: %v", msg.Key)
	}
	err := pub.(message.Publisher).Close()
	if err != nil {
		return fmt.Errorf("error closing publisher %v: %w", msg.Key, err)
	}
	w.logger.Debug("Closed publisher", watermill.LogFields{"key": msg.Key})
	return nil
}

func (w *WatermillMiddleware[K]) handleCloseSubscriber(ctx context.Context, msg *CloseSubscriberMessage[K]) error {
	defer close(msg.Response)
	sub, ok := w.subs.LoadAndDelete(msg.Key)
	if !ok {
		return fmt.Errorf("subscriber not found: %v", msg.Key)
	}
	err := sub.(message.Subscriber).Close()
	if err != nil {
		return fmt.Errorf("error closing subscriber %v: %w", msg.Key, err)
	}
	w.logger.Debug("Closed subscriber", watermill.LogFields{"key": msg.Key})
	return nil
}

type RouterStatus struct {
	Router  *message.Router
	Running bool
}

func (w *WatermillMiddleware[K]) handleAddRouter(ctx context.Context, msg *AddRouterMessage[K]) error {
	defer close(msg.Response)
	w.routers.Store(msg.Key, &RouterStatus{Router: msg.Router, Running: false})
	w.logger.Debug("Added router", watermill.LogFields{"key": msg.Key})

	go func() {
		if err := msg.Router.Run(ctx); err != nil {
			w.logger.Error("Error running router", err, watermill.LogFields{"key": msg.Key})
		}
	}()

	// Mark router as running
	if rs, ok := w.routers.Load(msg.Key); ok {
		rs.(*RouterStatus).Running = true
	}

	return nil
}

func (w *WatermillMiddleware[K]) handleAddHandler(ctx context.Context, msg *AddHandlerMessage[K]) error {
	defer close(msg.Response)
	routerStatus, ok := w.routers.Load(msg.Key)
	if !ok {
		return fmt.Errorf("router not found: %v", msg.Key)
	}
	rs := routerStatus.(*RouterStatus)

	if !rs.Running {
		return fmt.Errorf("router is not running: %v", msg.Key)
	}

	// Find the appropriate subscriber and publisher
	var sub message.Subscriber
	var pub message.Publisher

	w.subs.Range(func(key, value interface{}) bool {
		sub = value.(message.Subscriber)
		return false // Stop after finding the first subscriber
	})

	w.pubs.Range(func(key, value interface{}) bool {
		pub = value.(message.Publisher)
		return false // Stop after finding the first publisher
	})

	if sub == nil || pub == nil {
		return fmt.Errorf("subscriber or publisher not found")
	}

	rs.Router.AddHandler(
		msg.HandlerName,
		msg.SubscribeTopic,
		sub,
		msg.PublishTopic,
		pub,
		msg.HandlerFunc,
	)

	if err := rs.Router.RunHandlers(ctx); err != nil {
		return fmt.Errorf("error running handlers: %w", err)
	}

	w.logger.Debug("Added and ran handler", watermill.LogFields{
		"routerKey":   msg.Key,
		"handlerName": msg.HandlerName,
		"subTopic":    msg.SubscribeTopic,
		"pubTopic":    msg.PublishTopic,
	})
	return nil
}

func UsePublisher[K comparable](ctx context.Context, name K) (func(topic string, messages ...*message.Message) error, error) {
	middleware, err := getMiddleware[K](ctx)
	if err != nil {
		return nil, err
	}

	pubInterface, ok := middleware.pubs.Load(name)
	if !ok {
		return nil, fmt.Errorf("publisher not found: %v", name)
	}
	pub := pubInterface.(message.Publisher)

	return pub.Publish, nil
}

func UseSubscriber[K comparable](ctx context.Context, name K) (func(ctx context.Context, topic string) (<-chan *message.Message, error), error) {
	middleware, err := getMiddleware[K](ctx)
	if err != nil {
		return nil, err
	}

	subInterface, ok := middleware.subs.Load(name)
	if !ok {
		return nil, fmt.Errorf("subscriber not found: %v", name)
	}
	sub := subInterface.(message.Subscriber)

	return sub.Subscribe, nil
}

func getMiddleware[K comparable](ctx context.Context) (*WatermillMiddleware[K], error) {
	middleware, ok := ctx.Value(ctxWatermillKey).(*WatermillMiddleware[K])
	if !ok {
		return nil, fmt.Errorf("watermill middleware not found in context")
	}
	return middleware, nil
}
