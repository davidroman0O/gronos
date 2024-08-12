package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"sync"
	"syscall"
	"time"

	"github.com/ThreeDotsLabs/watermill"
	"github.com/ThreeDotsLabs/watermill/message"
	"github.com/ThreeDotsLabs/watermill/message/router/middleware"
	"github.com/ThreeDotsLabs/watermill/message/router/plugin"
	"github.com/ThreeDotsLabs/watermill/pubsub/gochannel"
	"github.com/davidroman0O/gronos"
	watermillextension "github.com/davidroman0O/gronos/watermill"
)

const (
	baseURL             = "https://api.binance.com/api/v3/klines"
	symbol              = "BTCUSDT"
	interval            = "1w"
	limit               = 1000
	topicFetchRequest   = "fetch_request"
	topicWeeklyPrices   = "weekly_prices"
	topicTradingSignals = "trading_signals"
	topicProcessedWeek  = "processed_week"
)

type WeeklyKline struct {
	OpenTime time.Time
	Open     float64
	High     float64
	Low      float64
	Close    float64
	Volume   float64
}

type TradingSignal struct {
	Signal string
	Price  float64
	Time   time.Time
	Reason string
}

type FetchRequest struct {
	StartTime time.Time
	EndTime   time.Time
}

type PriceManagerMiddleware struct {
	priceHistory []WeeklyKline
	mu           sync.Mutex
}

func NewPriceManagerMiddleware() *PriceManagerMiddleware {
	return &PriceManagerMiddleware{
		priceHistory: make([]WeeklyKline, 0),
	}
}

type AddKlineMessage struct {
	gronos.KeyMessage[string]
	Kline WeeklyKline
	gronos.RequestMessage[string, struct{}]
}

type GetPriceHistoryMessage struct {
	gronos.KeyMessage[string]
	gronos.RequestMessage[string, []WeeklyKline]
}

var addKlinePool sync.Pool
var getPriceHistoryPool sync.Pool

func init() {
	addKlinePool = sync.Pool{
		New: func() interface{} {
			return &AddKlineMessage{}
		},
	}
	getPriceHistoryPool = sync.Pool{
		New: func() interface{} {
			return &GetPriceHistoryMessage{}
		},
	}
}

func MsgAddKline(kline WeeklyKline) (<-chan struct{}, *AddKlineMessage) {
	msg := addKlinePool.Get().(*AddKlineMessage)
	msg.Key = "price_manager"
	msg.Kline = kline
	msg.Response = make(chan struct{}, 1)
	return msg.Response, msg
}

func MsgGetPriceHistory() (<-chan []WeeklyKline, *GetPriceHistoryMessage) {
	msg := getPriceHistoryPool.Get().(*GetPriceHistoryMessage)
	msg.Key = "price_manager"
	msg.Response = make(chan []WeeklyKline, 1)
	return msg.Response, msg
}

func (pm *PriceManagerMiddleware) OnStart(ctx context.Context, errChan chan<- error) error {
	return nil
}

func (pm *PriceManagerMiddleware) OnStop(ctx context.Context, errChan chan<- error) error {
	return nil
}

func (pm *PriceManagerMiddleware) OnNewRuntime(ctx context.Context) context.Context {
	return ctx
}

func (pm *PriceManagerMiddleware) OnStopRuntime(ctx context.Context) context.Context {
	return ctx
}

func (pm *PriceManagerMiddleware) OnMsg(ctx context.Context, m gronos.Message) error {
	switch msg := m.(type) {
	case *AddKlineMessage:
		pm.mu.Lock()
		pm.priceHistory = append(pm.priceHistory, msg.Kline)
		pm.mu.Unlock()
		close(msg.Response)
		addKlinePool.Put(msg)
		return nil
	case *GetPriceHistoryMessage:
		pm.mu.Lock()
		history := make([]WeeklyKline, len(pm.priceHistory))
		copy(history, pm.priceHistory)
		pm.mu.Unlock()
		msg.Response <- history
		close(msg.Response)
		getPriceHistoryPool.Put(msg)
		return nil
	default:
		return gronos.ErrUnmanageExtensionMessage
	}
}

func preparePublisherSubscriber(ctx context.Context, shutdown <-chan struct{}) error {
	wait, err := gronos.UseBusWait(ctx)
	if err != nil {
		return err
	}

	pubSub := gochannel.NewGoChannel(gochannel.Config{}, watermill.NewStdLogger(false, false))
	<-wait(func() (<-chan struct{}, gronos.Message) {
		return watermillextension.MsgAddPublisher("pubsub", pubSub)
	})

	<-wait(func() (<-chan struct{}, gronos.Message) {
		return watermillextension.MsgAddSubscriber("pubsub", pubSub)
	})

	return nil
}

func prepareRouter(ctx context.Context, shutdown <-chan struct{}) error {
	wait, err := gronos.UseBusWait(ctx)
	if err != nil {
		return err
	}

	router, err := message.NewRouter(message.RouterConfig{}, watermill.NewStdLogger(false, false))
	if err != nil {
		return err
	}

	<-wait(func() (<-chan struct{}, gronos.Message) {
		return watermillextension.MsgAddRouter("router", router)
	})

	return nil
}

func prepareRouterPluginsMiddlewares(ctx context.Context, shutdown <-chan struct{}) error {
	confirm, err := gronos.UseBusConfirm(ctx)
	if err != nil {
		return err
	}
	wait, err := gronos.UseBusWait(ctx)
	if err != nil {
		return err
	}

	for !<-confirm(func() (<-chan bool, gronos.Message) {
		return watermillextension.MsgHasPublisher("pubsub")
	}) {
		time.Sleep(100 * time.Millisecond)
	}

	publisher, err := watermillextension.UsePublisher(ctx, "pubsub")
	if err != nil {
		return err
	}

	poisonQueue, err := middleware.PoisonQueue(publisher, "poison_queue")
	if err != nil {
		panic(err)
	}

	<-wait(func() (<-chan struct{}, gronos.Message) {
		return watermillextension.MsgAddMiddlewares(
			"router",
			middleware.Recoverer,
			middleware.NewThrottle(10, time.Second).Middleware,
			poisonQueue,
			middleware.CorrelationID,
			middleware.Retry{
				MaxRetries:      1,
				InitialInterval: time.Millisecond * 10,
			}.Middleware,
		)
	})

	<-wait(func() (<-chan struct{}, gronos.Message) {
		return watermillextension.MsgAddPlugins(
			"router",
			plugin.SignalsHandler,
		)
	})

	return nil
}

func signalReadyness(ctx context.Context, shutdown <-chan struct{}) error {
	confirm, err := gronos.UseBusConfirm(ctx)
	if err != nil {
		panic(err)
	}

	for !<-confirm(func() (<-chan bool, gronos.Message) {
		return watermillextension.MsgHasPublisher("pubsub")
	}) {
		time.Sleep(100 * time.Millisecond)
	}

	for !<-confirm(func() (<-chan bool, gronos.Message) {
		return watermillextension.MsgHasSubscriber("pubsub")
	}) {
		time.Sleep(100 * time.Millisecond)
	}

	for !<-confirm(func() (<-chan bool, gronos.Message) {
		return watermillextension.MsgHasRouter("router")
	}) {
		time.Sleep(100 * time.Millisecond)
	}

	return nil
}

func getBinanceData(symbol, interval string, startTime, endTime int64) ([]WeeklyKline, error) {
	url := fmt.Sprintf("%s?symbol=%s&interval=%s&startTime=%d&endTime=%d&limit=%d", baseURL, symbol, interval, startTime, endTime, limit)

	resp, err := http.Get(url)
	if err != nil {
		return nil, fmt.Errorf("HTTP GET request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := ioutil.ReadAll(resp.Body)
		return nil, fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(body))
	}

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	var rawKlines [][]interface{}
	if err := json.Unmarshal(body, &rawKlines); err != nil {
		return nil, fmt.Errorf("failed to unmarshal JSON: %w\nBody: %s", err, string(body))
	}

	if len(rawKlines) == 0 {
		return nil, fmt.Errorf("no data returned from API")
	}

	klines := make([]WeeklyKline, len(rawKlines))
	for i, k := range rawKlines {
		if len(k) < 6 {
			return nil, fmt.Errorf("invalid kline data format at index %d: %v", i, k)
		}

		openTime := time.Unix(int64(k[0].(float64))/1000, 0)
		open, _ := strconv.ParseFloat(k[1].(string), 64)
		high, _ := strconv.ParseFloat(k[2].(string), 64)
		low, _ := strconv.ParseFloat(k[3].(string), 64)
		close, _ := strconv.ParseFloat(k[4].(string), 64)
		volume, _ := strconv.ParseFloat(k[5].(string), 64)

		klines[i] = WeeklyKline{
			OpenTime: openTime,
			Open:     open,
			High:     high,
			Low:      low,
			Close:    close,
			Volume:   volume,
		}
	}

	return klines, nil
}

func plannerWorker(ctx context.Context) error {
	publish, err := watermillextension.UsePublish(ctx, "pubsub")
	if err != nil {
		return err
	}

	subscribe, err := watermillextension.UseSubscribe(ctx, "pubsub")
	if err != nil {
		return err
	}

	processedWeeks, err := subscribe(ctx, topicProcessedWeek)
	if err != nil {
		return err
	}

	// Start from the oldest available data (e.g., 5 years ago)
	currentWeekStart := time.Now().AddDate(-5, 0, 0).Truncate(7 * 24 * time.Hour)
	processedWeekMap := make(map[time.Time]bool)

	for currentWeekStart.Before(time.Now()) {
		weekEndTime := currentWeekStart.Add(7 * 24 * time.Hour)

		if !processedWeekMap[currentWeekStart] {
			fetchRequest := FetchRequest{
				StartTime: currentWeekStart,
				EndTime:   weekEndTime,
			}

			payload, _ := json.Marshal(fetchRequest)
			msg := message.NewMessage(watermill.NewUUID(), payload)

			if err := publish(topicFetchRequest, msg); err != nil {

				return err
			}

			// Wait for the week to be processed
			select {
			case processedMsg := <-processedWeeks:

				processedMsg.Ack()
				processedWeekMap[currentWeekStart] = true
			case <-ctx.Done():
				return ctx.Err()
			}
		}

		currentWeekStart = weekEndTime
	}

	fmt.Println("All historical data processed.")
	return nil
}

func fetcherWorker(ctx context.Context, shutdown <-chan struct{}) error {
	subscribe, err := watermillextension.UseSubscribe(ctx, "pubsub")
	if err != nil {
		return err
	}

	publish, err := watermillextension.UsePublish(ctx, "pubsub")
	if err != nil {
		return err
	}

	fetchRequests, err := subscribe(ctx, topicFetchRequest)
	if err != nil {
		return err
	}

	for {
		select {
		case msg := <-fetchRequests:
			var request FetchRequest
			if err := json.Unmarshal(msg.Payload, &request); err != nil {

				continue
			}

			klines, err := getBinanceData(symbol, interval, request.StartTime.Unix()*1000, request.EndTime.Unix()*1000)
			if err != nil {

				// Publish an error message or handle the error appropriately
				errorPayload, _ := json.Marshal(map[string]string{"error": err.Error()})
				errorMsg := message.NewMessage(watermill.NewUUID(), errorPayload)
				if err := publish(topicProcessedWeek, errorMsg); err != nil {
					fmt.Printf("Error publishing error message: %v\n", err)
				}
				time.Sleep(5 * time.Second) // Wait before retrying
				continue
			}

			for _, kline := range klines {
				payload, _ := json.Marshal(kline)
				weeklyPriceMsg := message.NewMessage(watermill.NewUUID(), payload)
				if err := publish(topicWeeklyPrices, weeklyPriceMsg); err != nil {
					fmt.Printf("Error publishing weekly price: %v\n", err)
				}
			}

			// Publish a message indicating that the week has been processed
			processedMsg := message.NewMessage(watermill.NewUUID(), []byte("processed"))
			if err := publish(topicProcessedWeek, processedMsg); err != nil {
				fmt.Printf("Error publishing processed week message: %v\n", err)
			}

			msg.Ack()
			time.Sleep(1 * time.Second) // Rate limiting
		case <-ctx.Done():
			return ctx.Err()
		case <-shutdown:
			fmt.Println("Fetcher worker shutting down")
			return nil
		}
	}
}

func bitcoinPriceHandler(ctx context.Context) func(msg *message.Message) ([]*message.Message, error) {
	return func(msg *message.Message) ([]*message.Message, error) {
		var kline WeeklyKline
		if err := json.Unmarshal(msg.Payload, &kline); err != nil {

			return nil, err
		}

		send, err := gronos.UseBus(ctx)
		if err != nil {

			return nil, err
		}

		doneChan, addMsg := MsgAddKline(kline)
		if !send(addMsg) {

			return nil, fmt.Errorf("error sending AddKline message to price manager")
		}
		<-doneChan

		histChan, histMsg := MsgGetPriceHistory()
		if !send(histMsg) {

			return nil, fmt.Errorf("error sending GetPriceHistory message to price manager")
		}

		priceHistory := <-histChan

		signal := generateTradingSignal(priceHistory)

		messages := []*message.Message{}

		if signal != nil {
			payload, _ := json.Marshal(signal)
			signalMsg := message.NewMessage(watermill.NewUUID(), payload)
			messages = append(messages, signalMsg)

			fmt.Printf("Signal generated for %s: %s at $%.2f\nReason: %s\n",
				kline.OpenTime.Format("2006-01-02"), signal.Signal, signal.Price, signal.Reason)
		} else {
			fmt.Printf("No signal generated for %s\n", kline.OpenTime.Format("2006-01-02"))
		}

		fmt.Printf("Processed week: %s, Open: $%.2f, Close: $%.2f\n",
			kline.OpenTime.Format("2006-01-02"), kline.Open, kline.Close)

		// Notify that the week has been processed
		processedMsg := message.NewMessage(watermill.NewUUID(), []byte("processed"))
		messages = append(messages, processedMsg)

		return messages, nil
	}
}

func bitcoinSignalConsumer(ctx context.Context, shutdown <-chan struct{}) error {
	subscribe, err := watermillextension.UseSubscribe(ctx, "pubsub")
	if err != nil {
		return err
	}

	messages, err := subscribe(ctx, topicTradingSignals)
	if err != nil {
		return err
	}

	for {
		select {
		case msg := <-messages:
			var signal TradingSignal
			if err := json.Unmarshal(msg.Payload, &signal); err != nil {

				continue
			}
			fmt.Printf("Trading Signal: %s at $%.2f\nReason: %s\nTime: %s\n\n",
				signal.Signal, signal.Price, signal.Reason, signal.Time.Format(time.RFC3339))
			msg.Ack()
		case <-ctx.Done():
			return ctx.Err()
		case <-shutdown:
			return nil
		}
	}
}

func calculateEMA(priceHistory []WeeklyKline, period int) float64 {
	if len(priceHistory) < period {
		return 0
	}

	multiplier := 2.0 / float64(period+1)
	ema := priceHistory[len(priceHistory)-period].Close

	for i := len(priceHistory) - period + 1; i < len(priceHistory); i++ {
		ema = (priceHistory[i].Close-ema)*multiplier + ema
	}

	return ema
}

func generateTradingSignal(priceHistory []WeeklyKline) *TradingSignal {
	if len(priceHistory) == 0 {
		return nil
	}

	currentPrice := priceHistory[len(priceHistory)-1].Close
	ema20 := calculateEMA(priceHistory, 20)
	ema50 := calculateEMA(priceHistory, 50)

	var signal string
	var reason string

	if ema20 > ema50 && currentPrice > ema20 {
		signal = "BUY"
		reason = "Price above EMA20, EMA20 above EMA50 (Golden Cross)"
	} else if ema20 < ema50 && currentPrice < ema20 {
		signal = "SELL"
		reason = "Price below EMA20, EMA20 below EMA50 (Death Cross)"
	} else {
		signal = "HOLD"
		reason = "No clear signal"
	}

	return &TradingSignal{
		Signal: signal,
		Price:  currentPrice,
		Time:   priceHistory[len(priceHistory)-1].OpenTime,
		Reason: reason,
	}
}

func main() {
	logger := watermill.NewStdLogger(false, false)
	watermillExt := watermillextension.New[string](logger)

	ctx := context.Background()
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	priceManagerMiddleware := NewPriceManagerMiddleware()

	g, cerrs := gronos.New[string](
		ctx,
		map[string]gronos.RuntimeApplication{
			"setup-pubsub":                     preparePublisherSubscriber,
			"ready":                            signalReadyness,
			"setup-router":                     prepareRouter,
			"setup-router-plugins-middlewares": prepareRouterPluginsMiddlewares,
		},
		gronos.WithExtension[string](watermillExt),
		gronos.WithExtension[string](priceManagerMiddleware))

	go func() {
		for e := range cerrs {
			fmt.Println(e)
		}
	}()

	// Wait for readiness
	for !g.IsComplete("ready") {
		time.Sleep(100 * time.Millisecond)
	}

	// Set up the planner worker
	<-g.Add("planner-worker", gronos.Worker(1*time.Second, gronos.ManagedTimeline, plannerWorker))

	// Set up the fetcher worker
	<-g.Add("fetcher-worker", fetcherWorker)

	// Set up the price handler
	<-g.Add("price-handler", func(ctx context.Context, shutdown <-chan struct{}) error {
		wait, err := gronos.UseBusWait(ctx)
		if err != nil {
			return err
		}

		<-wait(func() (<-chan struct{}, gronos.Message) {
			return watermillextension.MsgAddHandler(
				"router",
				"bitcoin_price_handler",
				topicWeeklyPrices,
				"pubsub",
				topicTradingSignals,
				"pubsub",
				bitcoinPriceHandler(ctx),
			)
		})

		<-wait(func() (<-chan struct{}, gronos.Message) {
			return watermillextension.MsgAddNoPublisherHandler(
				"router",
				"processed_week_handler",
				topicProcessedWeek,
				"pubsub",
				func(msg *message.Message) error {
					return nil
				},
			)
		})

		select {
		case <-shutdown:
			return nil
		case <-ctx.Done():
			return nil
		}
	})

	// Set up the signal consumer
	<-g.Add("signal-consumer", bitcoinSignalConsumer)

	// Set up graceful shutdown
	go func() {
		c := make(chan os.Signal, 1)
		signal.Notify(c, os.Interrupt, syscall.SIGTERM)
		<-c
		fmt.Println("Shutting down...")
		g.Shutdown()
	}()

	// Wait for all components to finish
	g.Wait()
}
