package watermillextension

import (
	"context"
	"fmt"
	"sync"

	"github.com/ThreeDotsLabs/watermill"
	"github.com/ThreeDotsLabs/watermill/message"
	"github.com/davidroman0O/gronos"
)

/// The main goal of middlewares is to manage resources for the user

type ctxWatermill string

var ctxWatermillKey ctxWatermill

type WatermillExtension[K comparable] struct {
	pubs    sync.Map
	subs    sync.Map
	routers sync.Map
	logger  watermill.LoggerAdapter
}

func New[K comparable](logger watermill.LoggerAdapter) *WatermillExtension[K] {
	if logger == nil {
		logger = watermill.NewStdLogger(false, false)
	}
	return &WatermillExtension[K]{
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

type HasPublisherMessage[K comparable] struct {
	gronos.KeyMessage[K]
	gronos.RequestMessage[K, bool] // when it is done
}

type HasSubscriberMessage[K comparable] struct {
	gronos.KeyMessage[K]
	gronos.RequestMessage[K, bool] // when it is done
}

type AddRouterMessage[K comparable] struct {
	gronos.KeyMessage[K]
	Router                             *message.Router
	gronos.RequestMessage[K, struct{}] // when it is done
}

type HasRouterMessage[K comparable] struct {
	gronos.KeyMessage[K]
	gronos.RequestMessage[K, bool] // when it is done
}

type HasHandlerMessage[K comparable] struct {
	gronos.KeyMessage[K]
	HandlerName                    string
	gronos.RequestMessage[K, bool] // when it is done
}

type ClosePublisherMessage[K comparable] struct {
	gronos.KeyMessage[K]
	gronos.RequestMessage[K, struct{}] // when it is done
}

type CloseSubscriberMessage[K comparable] struct {
	gronos.KeyMessage[K]
	gronos.RequestMessage[K, struct{}] // when it is done
}

// func(handlerName string, subscribeTopic string, subscriber message.Subscriber, handlerFunc message.NoPublishHandlerFunc) *message.Handler
type AddNoPublisherHandlerMessage[K comparable] struct {
	gronos.KeyMessage[K]
	HandlerName                        string
	SubscribeTopic                     string
	SubscriberName                     string
	HandlerFunc                        message.NoPublishHandlerFunc
	gronos.RequestMessage[K, struct{}] // when it is done
}

type AddHandlerMessage[K comparable] struct {
	gronos.KeyMessage[K]
	HandlerName                        string
	SubscribeTopic                     string
	PublishTopic                       string
	SubscriberName                     string
	PublisherName                      string
	HandlerFunc                        message.HandlerFunc
	gronos.RequestMessage[K, struct{}] // when it is done
}

type AddRouterPlugins[K comparable] struct {
	gronos.KeyMessage[K]
	Plugins                            []message.RouterPlugin
	gronos.RequestMessage[K, struct{}] // when it is done
}

type AddRouterMiddlewares[K comparable] struct {
	gronos.KeyMessage[K]
	Middlewares                        []message.HandlerMiddleware
	gronos.RequestMessage[K, struct{}] // when it is done
}

// Sync pools for message types
var addPublisherPoolInited bool
var addPublisherPool sync.Pool

var addSubscriberPoolInited bool
var addSubscriberPool sync.Pool

var hasPublisherPoolInited bool
var hasPublisherPool sync.Pool

var hasSubscriberPoolInited bool
var hasSubscriberPool sync.Pool

var addRouterPoolInited bool
var addRouterPool sync.Pool

var hasRouterPoolInited bool
var hasRouterPool sync.Pool

var closePublisherPoolInited bool
var closePublisherPool sync.Pool

var closeSubscriberPoolInited bool
var closeSubscriberPool sync.Pool

var addHandlerPoolInited bool
var addHandlerPool sync.Pool

var hasHandlerPoolInited bool
var hasHandlerPool sync.Pool

var addNoPublisherHandlerPoolInited bool
var addNoPublisherHandlerPool sync.Pool

var addPluginsPoolInited bool
var addPluginsPool sync.Pool

var addMiddlewaresPoolInited bool
var addMiddlewaresPool sync.Pool

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

func MsgHasPublisher[K comparable](key K) (<-chan bool, *HasPublisherMessage[K]) {
	if !hasPublisherPoolInited {
		hasPublisherPoolInited = true
		hasPublisherPool = sync.Pool{
			New: func() interface{} {
				return &HasPublisherMessage[K]{}
			},
		}
	}
	msg := hasPublisherPool.Get().(*HasPublisherMessage[K])
	msg.Key = key
	msg.Response = make(chan bool, 1)
	return msg.Response, msg
}

func MsgHasSubscriber[K comparable](key K) (<-chan bool, *HasSubscriberMessage[K]) {
	if !hasSubscriberPoolInited {
		hasSubscriberPoolInited = true
		hasSubscriberPool = sync.Pool{
			New: func() interface{} {
				return &HasSubscriberMessage[K]{}
			},
		}
	}
	msg := hasSubscriberPool.Get().(*HasSubscriberMessage[K])
	msg.Key = key
	msg.Response = make(chan bool, 1)
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

func MsgAddHandler[K comparable](key K, handlerName, subscribeTopic, subscriberName string, publishTopic string, publisherName string, handlerFunc message.HandlerFunc) (<-chan struct{}, *AddHandlerMessage[K]) {
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
	msg.SubscriberName = subscriberName
	msg.PublisherName = publisherName
	msg.Response = make(chan struct{}, 1)
	return msg.Response, msg
}

func MsgHasRouter[K comparable](key K) (<-chan bool, *HasRouterMessage[K]) {
	if !hasRouterPoolInited {
		hasRouterPoolInited = true
		hasRouterPool = sync.Pool{
			New: func() interface{} {
				return &HasRouterMessage[K]{}
			},
		}
	}
	msg := hasRouterPool.Get().(*HasRouterMessage[K])
	msg.Key = key
	msg.Response = make(chan bool, 1)
	return msg.Response, msg
}

func MsgHasHandler[K comparable](key K, handlerName string) (<-chan bool, *HasHandlerMessage[K]) {
	if !hasHandlerPoolInited {
		hasHandlerPoolInited = true
		hasHandlerPool = sync.Pool{
			New: func() interface{} {
				return &HasHandlerMessage[K]{}
			},
		}
	}
	msg := hasHandlerPool.Get().(*HasHandlerMessage[K])
	msg.Key = key
	msg.HandlerName = handlerName
	msg.Response = make(chan bool, 1)
	return msg.Response, msg
}

// func(handlerName string, subscribeTopic string, subscriber message.Subscriber, handlerFunc message.NoPublishHandlerFunc) *message.Handler
func MsgAddNoPublisherHandler[K comparable](key K, handlerName, subscribeTopic, subscriberName string, handlerFunc message.NoPublishHandlerFunc) (<-chan struct{}, *AddNoPublisherHandlerMessage[K]) {
	if !addNoPublisherHandlerPoolInited {
		addNoPublisherHandlerPoolInited = true
		addNoPublisherHandlerPool = sync.Pool{
			New: func() interface{} {
				return &AddNoPublisherHandlerMessage[K]{}
			},
		}
	}
	msg := addNoPublisherHandlerPool.Get().(*AddNoPublisherHandlerMessage[K])
	msg.Key = key
	msg.HandlerName = handlerName
	msg.SubscribeTopic = subscribeTopic
	msg.HandlerFunc = handlerFunc
	msg.SubscriberName = subscriberName
	msg.Response = make(chan struct{}, 1)
	return msg.Response, msg
}

func MsgAddPlugins[K comparable](key K, plugins ...message.RouterPlugin) (<-chan struct{}, *AddRouterPlugins[K]) {
	if !addPluginsPoolInited {
		addPluginsPoolInited = true
		addPluginsPool = sync.Pool{
			New: func() interface{} {
				return &AddRouterPlugins[K]{}
			},
		}
	}
	msg := addPluginsPool.Get().(*AddRouterPlugins[K])
	msg.Key = key
	msg.Plugins = plugins
	msg.Response = make(chan struct{}, 1)
	return msg.Response, msg
}

func MsgAddRouterMiddlewares[K comparable](key K, middlewares ...message.HandlerMiddleware) (<-chan struct{}, *AddRouterMiddlewares[K]) {
	if !addMiddlewaresPoolInited {
		addMiddlewaresPoolInited = true
		addMiddlewaresPool = sync.Pool{
			New: func() interface{} {
				return &AddRouterMiddlewares[K]{}
			},
		}
	}
	msg := addMiddlewaresPool.Get().(*AddRouterMiddlewares[K])
	msg.Key = key
	msg.Middlewares = middlewares
	msg.Response = make(chan struct{}, 1)
	return msg.Response, msg
}

// Extension methods
func (w *WatermillExtension[K]) OnStart(ctx context.Context, errChan chan<- error) error {
	w.logger.Debug("Starting Watermill middleware", nil)
	return nil
}

func (w *WatermillExtension[K]) OnNewRuntime(ctx context.Context) context.Context {
	return context.WithValue(ctx, ctxWatermillKey, w)
}

func (w *WatermillExtension[K]) OnStopRuntime(ctx context.Context) context.Context {
	return ctx
}

func (w *WatermillExtension[K]) OnStop(ctx context.Context, errChan chan<- error) error {
	w.logger.Debug("Stopping Watermill middleware", nil)
	return w.closeAllComponents(errChan)
}

func (w *WatermillExtension[K]) closeAllComponents(errChan chan<- error) error {
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

	for err := range errCh {
		errChan <- err
	}

	return nil
}

func (w *WatermillExtension[K]) OnMsg(ctx context.Context, m *gronos.MessagePayload) error {
	switch msg := m.Message.(type) {
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
	case *AddNoPublisherHandlerMessage[K]:
		defer addNoPublisherHandlerPool.Put(msg)
		return w.handleAddNoPublisherHandler(ctx, msg)
	case *AddRouterPlugins[K]:
		defer addPluginsPool.Put(msg)
		return w.handleAddRouterPlugins(ctx, msg)
	case *AddRouterMiddlewares[K]:
		defer addMiddlewaresPool.Put(msg)
		return w.handleAddRouterMiddlewares(ctx, msg)
	case *HasPublisherMessage[K]:
		defer hasPublisherPool.Put(msg)
		return w.handleHasPublisher(ctx, msg)
	case *HasSubscriberMessage[K]:
		defer hasSubscriberPool.Put(msg)
		return w.handleHasSubscriber(ctx, msg)
	case *HasHandlerMessage[K]:
		defer hasHandlerPool.Put(msg)
		return w.handleHasHandler(ctx, msg)
	case *HasRouterMessage[K]:
		defer hasRouterPool.Put(msg)
		return w.handleHasRouter(ctx, msg)
	default:
		return gronos.ErrUnmanageExtensionMessage
	}
}

func (w *WatermillExtension[K]) handleHasPublisher(ctx context.Context, msg *HasPublisherMessage[K]) error {
	defer close(msg.Response)
	_, ok := w.pubs.Load(msg.Key)
	msg.Response <- ok
	return nil
}

func (w *WatermillExtension[K]) handleHasSubscriber(ctx context.Context, msg *HasSubscriberMessage[K]) error {
	defer close(msg.Response)
	_, ok := w.subs.Load(msg.Key)
	msg.Response <- ok
	return nil
}

func (w *WatermillExtension[K]) handleHasHandler(ctx context.Context, msg *HasHandlerMessage[K]) error {
	defer close(msg.Response)
	var value any
	var ok bool
	value, ok = w.routers.Load(msg.Key)
	if !ok {
		msg.Response <- false
	}
	router := value.(*message.Router)
	_, ok = router.Handlers()[msg.HandlerName]
	msg.Response <- ok
	return nil
}

func (w *WatermillExtension[K]) handleHasRouter(ctx context.Context, msg *HasRouterMessage[K]) error {
	defer close(msg.Response)
	_, ok := w.routers.Load(msg.Key)
	msg.Response <- ok
	return nil
}

func (w *WatermillExtension[K]) handleAddPublisher(ctx context.Context, msg *AddPublisherMessage[K]) error {
	defer close(msg.Response)
	w.pubs.Store(msg.Key, msg.Publisher)
	w.logger.Debug("Added publisher", watermill.LogFields{"key": msg.Key})
	return nil
}

func (w *WatermillExtension[K]) handleAddSubscriber(ctx context.Context, msg *AddSubscriberMessage[K]) error {
	defer close(msg.Response)
	w.subs.Store(msg.Key, msg.Subscriber)
	w.logger.Debug("Added subscriber", watermill.LogFields{"key": msg.Key})
	return nil
}

func (w *WatermillExtension[K]) handleClosePublisher(ctx context.Context, msg *ClosePublisherMessage[K]) error {
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

func (w *WatermillExtension[K]) handleCloseSubscriber(ctx context.Context, msg *CloseSubscriberMessage[K]) error {
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

func (w *WatermillExtension[K]) handleAddRouter(ctx context.Context, msg *AddRouterMessage[K]) error {
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

func (w *WatermillExtension[K]) handleAddNoPublisherHandler(ctx context.Context, msg *AddNoPublisherHandlerMessage[K]) error {
	defer close(msg.Response)
	routerStatus, ok := w.routers.Load(msg.Key)
	if !ok {
		return fmt.Errorf("router not found: %v", msg.Key)
	}
	rs := routerStatus.(*RouterStatus)

	if !rs.Running {
		return fmt.Errorf("router is not running: %v", msg.Key)
	}

	// Find the appropriate subscriber
	var sub message.Subscriber
	var value any

	if value, ok = w.subs.Load(msg.SubscriberName); !ok {
		return fmt.Errorf("subscriber not found: %v", msg.SubscriberName)
	}
	sub = value.(message.Subscriber)

	if sub == nil {
		return fmt.Errorf("subscriber not found")
	}

	rs.Router.AddNoPublisherHandler(
		msg.HandlerName,
		msg.SubscribeTopic,
		sub,
		msg.HandlerFunc,
	)

	if err := rs.Router.RunHandlers(ctx); err != nil {
		return fmt.Errorf("error running handlers: %w", err)
	}

	w.logger.Debug("Added and ran no-publisher handler", watermill.LogFields{
		"routerKey":   msg.Key,
		"handlerName": msg.HandlerName,
		"subTopic":    msg.SubscribeTopic,
	})
	return nil
}

func (w *WatermillExtension[K]) handleAddRouterPlugins(ctx context.Context, msg *AddRouterPlugins[K]) error {
	defer close(msg.Response)
	routerStatus, ok := w.routers.Load(msg.Key)
	if !ok {
		return fmt.Errorf("router not found: %v", msg.Key)
	}
	rs := routerStatus.(*RouterStatus)

	if !rs.Running {
		return fmt.Errorf("router is not running: %v", msg.Key)
	}

	rs.Router.AddPlugin(msg.Plugins...)

	w.logger.Debug("Added plugins to router", watermill.LogFields{
		"routerKey": msg.Key,
		"plugins":   msg.Plugins,
	})
	return nil
}

func (w *WatermillExtension[K]) handleAddRouterMiddlewares(ctx context.Context, msg *AddRouterMiddlewares[K]) error {
	defer close(msg.Response)
	routerStatus, ok := w.routers.Load(msg.Key)
	if !ok {
		return fmt.Errorf("router not found: %v", msg.Key)
	}
	rs := routerStatus.(*RouterStatus)

	if !rs.Running {
		return fmt.Errorf("router is not running: %v", msg.Key)
	}

	rs.Router.AddMiddleware(msg.Middlewares...)

	w.logger.Debug("Added Middlewares to router", watermill.LogFields{
		"routerKey":   msg.Key,
		"middlewares": msg.Middlewares,
	})
	return nil
}

func (w *WatermillExtension[K]) handleAddHandler(ctx context.Context, msg *AddHandlerMessage[K]) error {
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
	var value any

	if value, ok = w.subs.Load(msg.SubscriberName); !ok {
		return fmt.Errorf("subscriber not found: %v", msg.SubscriberName)
	}
	sub = value.(message.Subscriber)

	if value, ok = w.pubs.Load(msg.PublisherName); !ok {
		return fmt.Errorf("publisher not found: %v", msg.SubscriberName)
	}
	pub = value.(message.Publisher)

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

func UsePublish[K comparable](ctx context.Context, name K) (func(topic string, messages ...*message.Message) error, error) {
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

func UseSubscribe[K comparable](ctx context.Context, name K) (func(ctx context.Context, topic string) (<-chan *message.Message, error), error) {
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

func UsePublisher[K comparable](ctx context.Context, name K) (message.Publisher, error) {
	middleware, err := getMiddleware[K](ctx)
	if err != nil {
		return nil, err
	}

	pubInterface, ok := middleware.pubs.Load(name)
	if !ok {
		return nil, fmt.Errorf("publisher not found: %v", name)
	}
	pub := pubInterface.(message.Publisher)

	return pub, nil
}

func UseSubscriber[K comparable](ctx context.Context, name K) (message.Subscriber, error) {
	middleware, err := getMiddleware[K](ctx)
	if err != nil {
		return nil, err
	}

	subInterface, ok := middleware.subs.Load(name)
	if !ok {
		return nil, fmt.Errorf("subscriber not found: %v", name)
	}
	sub := subInterface.(message.Subscriber)

	return sub, nil
}

func getMiddleware[K comparable](ctx context.Context) (*WatermillExtension[K], error) {
	middleware, ok := ctx.Value(ctxWatermillKey).(*WatermillExtension[K])
	if !ok {
		return nil, fmt.Errorf("watermill middleware not found in context")
	}
	return middleware, nil
}
