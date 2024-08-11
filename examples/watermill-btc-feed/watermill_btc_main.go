package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"os/signal"
	"sort"
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
	"golang.org/x/exp/rand"
)

const (
	baseURL             = "https://api.binance.com/api/v3/klines"
	symbol              = "BTCUSDT"
	interval            = "1w"
	limit               = 1000
	topicWeeklyPrices   = "weekly_prices"
	topicTradingSignals = "trading_signals"
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

// Add this function to get a random time between two times
func getRandomTimeBetween(start, end time.Time) time.Time {
	delta := end.Sub(start)
	randomDelta := time.Duration(rand.Int63n(int64(delta)))
	return start.Add(randomDelta)
}

type PriceManagerMiddleware struct {
	priceHistory []WeeklyKline
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

type ComputeSignalMessage struct {
	gronos.KeyMessage[string]
	Timestamp time.Time
	gronos.RequestMessage[string, *TradingSignal]
}

var addKlinePool sync.Pool
var getPriceHistoryPool sync.Pool
var computeSignalPool sync.Pool

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
	computeSignalPool = sync.Pool{
		New: func() interface{} {
			return &ComputeSignalMessage{}
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

func MsgComputeSignal(timestamp time.Time) (<-chan *TradingSignal, *ComputeSignalMessage) {
	msg := computeSignalPool.Get().(*ComputeSignalMessage)
	msg.Key = "price_manager"
	msg.Timestamp = timestamp
	msg.Response = make(chan *TradingSignal, 1)
	return msg.Response, msg
}

func (pm *PriceManagerMiddleware) OnStart(ctx context.Context, errChan chan<- error) error {
	historicalData, err := fetchHistoricalData(ctx)
	if err != nil {
		return fmt.Errorf("error fetching historical data: %v", err)
	}
	pm.priceHistory = historicalData
	fmt.Printf("Loaded %d historical klines. Latest historical price: $%.2f\n",
		len(historicalData), historicalData[len(historicalData)-1].Close)
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
		pm.priceHistory = append(pm.priceHistory, msg.Kline)
		close(msg.Response)
		addKlinePool.Put(msg)
		return nil
	case *GetPriceHistoryMessage:
		history := make([]WeeklyKline, len(pm.priceHistory))
		copy(history, pm.priceHistory)
		msg.Response <- history
		close(msg.Response)
		getPriceHistoryPool.Put(msg)
		return nil
	case *ComputeSignalMessage:
		history := pm.getPriceHistoryUntil(msg.Timestamp)
		signal := generateTradingSignal(history)
		msg.Response <- signal
		close(msg.Response)
		computeSignalPool.Put(msg)
		return nil
	default:
		return gronos.ErrUnmanageExtensionMessage
	}
}

func (pm *PriceManagerMiddleware) getPriceHistoryUntil(timestamp time.Time) []WeeklyKline {
	var history []WeeklyKline
	for _, kline := range pm.priceHistory {
		if kline.OpenTime.After(timestamp) {
			break
		}
		history = append(history, kline)
	}
	return history
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
		return nil, err
	}
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var rawKlines [][]interface{}
	if err := json.Unmarshal(body, &rawKlines); err != nil {
		return nil, err
	}

	klines := make([]WeeklyKline, len(rawKlines))
	for i, k := range rawKlines {
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

func fetchHistoricalData(ctx context.Context) ([]WeeklyKline, error) {
	endTime := time.Now().Unix() * 1000
	startTime := endTime - (200 * 7 * 24 * 60 * 60 * 1000) // 200 weeks

	klines, err := getBinanceData(symbol, interval, startTime, endTime)
	if err != nil {
		return nil, err
	}

	sort.Slice(klines, func(i, j int) bool {
		return klines[i].OpenTime.Before(klines[j].OpenTime)
	})

	return klines, nil
}

func bitcoinPriceProducer(ctx context.Context) error {
	publish, err := watermillextension.UsePublish(ctx, "pubsub")
	if err != nil {
		return err
	}

	wait, err := gronos.UseBusWait(ctx)
	if err != nil {
		return err
	}

	histChan, histMsg := MsgGetPriceHistory()
	<-wait(func() (<-chan struct{}, gronos.Message) {
		return nil, histMsg
	})

	priceHistory := <-histChan

	latestTime := time.Now().Add(-7*24*time.Hour).Unix() * 1000
	if len(priceHistory) > 0 {
		latestTime = priceHistory[len(priceHistory)-1].OpenTime.Unix() * 1000
	}

	klines, err := getBinanceData(symbol, interval, latestTime, time.Now().Unix()*1000)
	if err != nil {
		return err
	}

	for _, kline := range klines {
		doneChan, addMsg := MsgAddKline(kline)
		<-wait(func() (<-chan struct{}, gronos.Message) {
			return doneChan, addMsg
		})

		payload, _ := json.Marshal(kline)
		msg := message.NewMessage(watermill.NewUUID(), payload)
		if err := publish(topicWeeklyPrices, msg); err != nil {
			fmt.Printf("Error publishing message: %v\n", err)
		}

		// Compute and print signal for this kline
		signalChan, computeMsg := MsgComputeSignal(kline.OpenTime)
		<-wait(func() (<-chan struct{}, gronos.Message) {
			return nil, computeMsg
		})
		signal := <-signalChan
		if signal != nil {
			fmt.Printf("Computed Signal for %s: %s at $%.2f\nReason: %s\n\n",
				kline.OpenTime.Format(time.RFC3339), signal.Signal, signal.Price, signal.Reason)
		}
	}

	if len(klines) > 0 {
		fmt.Printf("Updated with %d new klines. Latest price: $%.2f\n", len(klines), klines[len(klines)-1].Close)
	}

	return nil
}

func bitcoinPriceHandler(msg *message.Message) ([]*message.Message, error) {
	var kline WeeklyKline
	if err := json.Unmarshal(msg.Payload, &kline); err != nil {
		return nil, err
	}

	wait, err := gronos.UseBusWait(msg.Context())
	if err != nil {
		return nil, err
	}

	doneChan, addMsg := MsgAddKline(kline)
	<-wait(func() (<-chan struct{}, gronos.Message) {
		return doneChan, addMsg
	})

	histChan, histMsg := MsgGetPriceHistory()
	<-wait(func() (<-chan struct{}, gronos.Message) {
		return nil, histMsg
	})

	priceHistory := <-histChan

	signal := generateTradingSignal(priceHistory)

	if signal != nil && signal.Signal != "HOLD" {
		payload, _ := json.Marshal(signal)
		signalMsg := message.NewMessage(watermill.NewUUID(), payload)
		return []*message.Message{signalMsg}, nil
	}

	return nil, nil
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
		return nil // Don't generate HOLD signals
	}

	return &TradingSignal{
		Signal: signal,
		Price:  currentPrice,
		Time:   priceHistory[len(priceHistory)-1].OpenTime,
		Reason: reason,
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
				fmt.Printf("Error unmarshaling signal message: %v\n", err)
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

	// Set up the price producer worker
	<-g.Add("price-producer", gronos.Worker(1*time.Minute, gronos.ManagedTimeline, bitcoinPriceProducer))

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
				bitcoinPriceHandler,
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

	// Set up periodic signal computation with random historical date
	<-g.Add("periodic-signal-computer", gronos.Worker(2*time.Second, gronos.ManagedTimeline, func(ctx context.Context) error {
		wait, err := gronos.UseBusWait(ctx)
		if err != nil {
			return err
		}

		// Get the current price history
		histChan, histMsg := MsgGetPriceHistory()
		<-wait(func() (<-chan struct{}, gronos.Message) {
			return nil, histMsg
		})
		priceHistory := <-histChan

		if len(priceHistory) < 2 {
			fmt.Println("Not enough historical data for random computation")
			return nil
		}

		// Get the first and last dates from the price history
		firstDate := priceHistory[0].OpenTime
		lastDate := priceHistory[len(priceHistory)-1].OpenTime

		// Generate a random date between the first and last dates
		randomDate := getRandomTimeBetween(firstDate, lastDate)

		fmt.Println("Computing signal for random historical date:", randomDate.Format(time.RFC3339))

		signalChan, computeMsg := MsgComputeSignal(randomDate)
		<-wait(func() (<-chan struct{}, gronos.Message) {
			return nil, computeMsg
		})
		signal := <-signalChan
		if signal != nil {
			fmt.Printf("Random Historical Signal Computation (%s): %s at $%.2f\nReason: %s\n\n",
				randomDate.Format(time.RFC3339), signal.Signal, signal.Price, signal.Reason)
		} else {
			fmt.Printf("No signal generated for random date %s\n\n", randomDate.Format(time.RFC3339))
		}

		// Also compute for the current time
		// now := time.Now()
		// signalChan, computeMsg = MsgComputeSignal(now)
		// <-wait(func() (<-chan struct{}, gronos.Message) {
		// 	return nil, computeMsg
		// })
		// signal = <-signalChan
		// if signal != nil {
		// 	fmt.Printf("Current Signal Computation (%s): %s at $%.2f\nReason: %s\n\n",
		// 		now.Format(time.RFC3339), signal.Signal, signal.Price, signal.Reason)
		// } else {
		// 	fmt.Printf("No signal generated for current time %s\n\n", now.Format(time.RFC3339))
		// }

		return nil
	}))

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
