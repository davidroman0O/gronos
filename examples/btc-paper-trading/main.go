package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/davidroman0O/comfylite3"
	"github.com/davidroman0O/gronos"
)

const (
	symbol              = "BTCUSDT"
	baseURL             = "https://api.binance.com/api/v3/klines"
	maxKlinesPerRequest = 1000
)

type Config struct {
	WeeksToFetch int
	RateLimit    time.Duration
}

type BinanceKline struct {
	OpenTime  int64
	Open      float64
	High      float64
	Low       float64
	Close     float64
	Volume    float64
	CloseTime int64
}

var intervals = []string{"1m", "5m", "15m", "30m", "1h", "4h", "1d", "1w", "1M"}

var intervalConfigs = map[string]time.Duration{
	"1m":  time.Minute,
	"5m":  5 * time.Minute,
	"15m": 15 * time.Minute,
	"30m": 30 * time.Minute,
	"1h":  time.Hour,
	"4h":  4 * time.Hour,
	"1d":  24 * time.Hour,
	"1w":  7 * 24 * time.Hour,
	"1M":  30 * 24 * time.Hour,
}

func init() {
	log.SetOutput(os.Stdout)
	log.SetFlags(log.Ldate | log.Ltime | log.Lmicroseconds | log.Lshortfile)
}

func initDatabase() (*comfylite3.ComfyDB, error) {
	log.Println("Initializing database...")
	migrations := []comfylite3.Migration{
		comfylite3.NewMigration(
			1,
			"create_btc_price_data_table",
			func(tx *sql.Tx) error {
				log.Println("Creating btc_price_data table...")
				_, err := tx.Exec(`
					CREATE TABLE btc_price_data (
						timestamp INTEGER NOT NULL,
						interval TEXT NOT NULL,
						open REAL,
						high REAL,
						low REAL,
						close REAL,
						volume REAL,
						fetched BOOLEAN NOT NULL DEFAULT 0,
						PRIMARY KEY (timestamp, interval)
					)
				`)
				if err != nil {
					log.Printf("Error creating table: %v", err)
				} else {
					log.Println("Table created successfully")
				}
				return err
			},
			func(tx *sql.Tx) error {
				log.Println("Dropping btc_price_data table...")
				_, err := tx.Exec("DROP TABLE IF EXISTS btc_price_data")
				if err != nil {
					log.Printf("Error dropping table: %v", err)
				} else {
					log.Println("Table dropped successfully")
				}
				return err
			},
		),
	}

	db, err := comfylite3.New(
		comfylite3.WithPath("btc_game.db"),
		comfylite3.WithMigration(migrations...),
	)
	if err != nil {
		log.Printf("Error initializing database: %v", err)
		return nil, err
	}

	if err := db.Up(context.Background()); err != nil {
		log.Printf("Error running migrations: %v", err)
		return nil, err
	}

	log.Println("Database initialized successfully")
	return db, nil
}

func alignToInterval(t time.Time, interval time.Duration) time.Time {
	// Aligns the timestamp to the nearest interval
	aligned := t.Truncate(interval)
	return aligned
}

func startOfMonth(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, t.Location())
}

func prepopulateKlines(db *comfylite3.ComfyDB, interval string, startTime, endTime time.Time) error {
	log.Printf("Prepopulating klines for interval %s from %v to %v", interval, startTime, endTime)

	var duration time.Duration
	if interval == "1M" {
		duration = time.Hour * 24 * 30 // Approximate month duration, adjust as needed
	} else {
		duration = intervalConfigs[interval]
	}

	stmt, err := db.Prepare(`
        INSERT OR IGNORE INTO btc_price_data (timestamp, interval, fetched)
        VALUES (?, ?, 0)
    `)
	if err != nil {
		return fmt.Errorf("error preparing statement: %w", err)
	}
	defer stmt.Close()

	fmt.Printf("Prepopulating klines for interval %s from %v to %v\n", interval, startTime, endTime)
	count := 0
	for t := startTime; t.Before(endTime); t = t.Add(duration) {
		var alignedTime time.Time
		if interval == "1M" {
			alignedTime = startOfMonth(t)
		} else {
			alignedTime = t
			alignedTime = alignToInterval(t, duration)
		}
		timestamp := alignedTime.UnixMilli() // Convert to milliseconds

		// Log the exact values being used in the update
		log.Printf("Attempting to pre-populate: timestamp=%d, interval=%s", timestamp, interval)

		_, err := stmt.Exec(timestamp, interval)
		if err != nil {
			return fmt.Errorf("error inserting kline for %s at %v: %w", interval, t, err)
		}
		count++
	}

	log.Printf("Prepopulated %d klines for interval %s", count, interval)
	return nil
}

func getUnfetchedKlineRanges(db *comfylite3.ComfyDB, interval string) ([][2]int64, error) {
	rows, err := db.Query(`
        SELECT timestamp
        FROM btc_price_data
        WHERE interval = ? AND fetched = 0
        ORDER BY timestamp ASC
        LIMIT 1000
    `, interval)
	if err != nil {
		return nil, fmt.Errorf("error querying unfetched klines: %w", err)
	}
	defer rows.Close()

	var timestamps []int64
	for rows.Next() {
		var ts int64
		if err := rows.Scan(&ts); err != nil {
			return nil, fmt.Errorf("error scanning timestamp: %w", err)
		}
		timestamps = append(timestamps, ts)
	}

	if len(timestamps) == 0 {
		return nil, nil
	}

	log.Printf("Found %d unfetched timestamps for interval %s", len(timestamps), interval)

	// Return a single range with the first and last timestamp
	return [][2]int64{{timestamps[0], timestamps[len(timestamps)-1]}}, nil
}

func fetchAndStoreKlines(db *comfylite3.ComfyDB, interval string, start, end int64) error {
	klines, err := getBinanceData(symbol, interval, start, end)
	if err != nil {
		return fmt.Errorf("error fetching data from Binance: %w", err)
	}

	if len(klines) == 0 {
		log.Printf("No klines fetched for %s, skipping update", interval)
		return nil
	}

	err = updateKlines(db, interval, klines)
	if err != nil {
		log.Printf("Error updating klines for %s: %v", interval, err)
		return err
	}

	log.Printf("Successfully updated %d klines for %s", len(klines), interval)
	return nil
}

func workerFunction(ctx context.Context, db *comfylite3.ComfyDB, interval string) error {
	ranges, err := getUnfetchedKlineRanges(db, interval)
	if err != nil {
		log.Printf("Error getting unfetched kline ranges for %s: %v", interval, err)
		return err
	}

	if len(ranges) == 0 {
		log.Printf("No unfetched klines for interval %s, terminating worker", interval)

		wait, err := gronos.UseBusWait(ctx)
		if err != nil {
			return fmt.Errorf("error using bus wait: %w", err)
		}

		<-wait(func() (<-chan struct{}, gronos.Message) {
			return gronos.MsgForceTerminateShutdown(fmt.Sprintf("worker-%s", interval))
		})

		return nil
	}

	log.Printf("Found %d unfetched ranges for interval %s", len(ranges), interval)

	for _, r := range ranges {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			// Log using the correct conversion from milliseconds to time.Time
			log.Printf("Fetching and storing klines for %s from %v to %v", interval, time.UnixMilli(r[0]), time.UnixMilli(r[1]))
			if err := fetchAndStoreKlines(db, interval, r[0], r[1]); err != nil {
				log.Printf("Error fetching and storing klines for %s: %v", interval, err)
				return err
			}
		}
	}

	log.Printf("Completed fetching and storing klines for interval %s", interval)
	return nil
}

func CreateRunner(db *comfylite3.ComfyDB, config Config) gronos.RuntimeApplication {
	return func(ctx context.Context, shutdown <-chan struct{}) error {
		log.Println("Starting BTC data fetcher")
		endTime := time.Now().UTC()
		startTime := endTime.AddDate(0, 0, -7*config.WeeksToFetch)
		log.Printf("Fetch period: %v to %v", startTime, endTime)

		for _, interval := range intervals {
			if err := prepopulateKlines(db, interval, startTime, endTime); err != nil {
				return fmt.Errorf("failed to prepopulate klines for %s: %w", interval, err)
			}
		}

		var wg sync.WaitGroup

		wait, err := gronos.UseBusWait(ctx)
		if err != nil {
			return fmt.Errorf("error using bus wait: %w", err)
		}

		for _, interval := range intervals {
			wg.Add(1)
			go func(interval string) {
				defer wg.Done()
				worker := gronos.Worker(time.Second, gronos.NonBlocking, func(ctx context.Context) error {
					return workerFunction(ctx, db, interval)
				})

				workerKey := fmt.Sprintf("worker-%s", interval)
				<-wait(func() (<-chan struct{}, gronos.Message) {
					return gronos.MsgAdd(workerKey, worker)
				})

				log.Printf("Worker for interval %s added", interval)

				<-wait(func() (<-chan struct{}, gronos.Message) {
					return gronos.MsgRequestStatusAsync(workerKey, gronos.StatusShutdownTerminated)
				})

				log.Printf("Worker for interval %s finished", interval)
			}(interval)
		}

		done := make(chan struct{})
		go func() {
			wg.Wait()
			close(done)
		}()

		select {
		case <-ctx.Done():
			log.Println("Context cancelled, stopping workers")
		case <-shutdown:
			log.Println("Shutdown signal received, stopping workers")
		case <-done:
			log.Println("All workers completed successfully")
		}

		wg.Wait()

		log.Println("Validating all fetched data...")
		if err := validateAllData(db, intervals, startTime, endTime); err != nil {
			log.Printf("Final validation failed: %v", err)
			return err
		}
		log.Println("All data validated successfully.")

		return nil
	}
}

func getBinanceData(symbol, interval string, startTime, endTime int64) ([]BinanceKline, error) {
	url := fmt.Sprintf("%s?symbol=%s&interval=%s&startTime=%d&endTime=%d&limit=1000", baseURL, symbol, interval, startTime, endTime)
	log.Printf("Fetching data from Binance: %s", url)

	resp, err := http.Get(url)
	if err != nil {
		log.Printf("Error making GET request: %v", err)
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Printf("Error reading response body: %v", err)
		return nil, err
	}

	var rawKlines [][]interface{}
	if err := json.Unmarshal(body, &rawKlines); err != nil {
		log.Printf("Error unmarshaling JSON: %v", err)
		return nil, err
	}

	klines := make([]BinanceKline, 0, len(rawKlines))
	for _, k := range rawKlines {
		kline, err := parseBinanceKline(k)
		if err != nil {
			log.Printf("Warning: %v", err)
			continue
		}
		klines = append(klines, kline)
	}

	log.Printf("Fetched %d klines from Binance", len(klines))
	return klines, nil
}

func parseBinanceKline(k []interface{}) (BinanceKline, error) {
	if len(k) < 7 {
		return BinanceKline{}, fmt.Errorf("insufficient data in kline")
	}

	openTime, _ := k[0].(float64)
	open, _ := strconv.ParseFloat(k[1].(string), 64)
	high, _ := strconv.ParseFloat(k[2].(string), 64)
	low, _ := strconv.ParseFloat(k[3].(string), 64)
	close, _ := strconv.ParseFloat(k[4].(string), 64)
	volume, _ := strconv.ParseFloat(k[5].(string), 64)
	closeTime, _ := k[6].(float64)

	return BinanceKline{
		OpenTime:  int64(openTime),
		Open:      open,
		High:      high,
		Low:       low,
		Close:     close,
		Volume:    volume,
		CloseTime: int64(closeTime),
	}, nil
}

func updateKlines(db *comfylite3.ComfyDB, interval string, klines []BinanceKline) error {
	log.Printf("Updating %d klines for interval %s", len(klines), interval)
	tx, err := db.Begin()
	if err != nil {
		log.Printf("Error beginning transaction: %v", err)
		return err
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(`
        UPDATE btc_price_data
        SET open = ?, high = ?, low = ?, close = ?, volume = ?, fetched = 1
        WHERE timestamp = ? AND interval = ?
    `)
	if err != nil {
		log.Printf("Error preparing update statement: %v", err)
		return err
	}
	defer stmt.Close()

	updatedCount := 0
	for _, kline := range klines {
		timestamp := kline.OpenTime // Already in milliseconds

		if interval == "1M" {
			// Log the exact values being used in the update
			log.Printf("Attempting to update: timestamp=%d, interval=%s", timestamp, interval)
		}

		result, err := stmt.Exec(kline.Open, kline.High, kline.Low, kline.Close, kline.Volume, timestamp, interval)
		if err != nil {
			log.Printf("Error updating kline for %s at %v: %v", interval, time.UnixMilli(timestamp), err)
			return err
		}
		rowsAffected, _ := result.RowsAffected()
		if interval == "1M" {
			log.Printf("Rows affected: %d", rowsAffected)
		}
		updatedCount += int(rowsAffected)
	}

	if err := tx.Commit(); err != nil {
		log.Printf("Error committing transaction: %v", err)
		return err
	}

	log.Printf("Successfully updated %d out of %d klines for interval %s", updatedCount, len(klines), interval)
	return nil
}

func validateAllData(db *comfylite3.ComfyDB, intervals []string, startTime, endTime time.Time) error {
	log.Println("Starting data validation for all intervals")
	for _, interval := range intervals {
		log.Printf("Validating data for interval: %s", interval)
		count, err := validateIntervalData(db, interval, startTime, endTime)
		if err != nil {
			log.Printf("Validation failed for %s: %v", interval, err)
			return fmt.Errorf("validation failed for %s: %w", interval, err)
		}
		log.Printf("Validated %d klines for %s", count, interval)
	}
	log.Println("Data validation completed successfully for all intervals")
	return nil
}

func validateIntervalData(db *comfylite3.ComfyDB, interval string, startTime, endTime time.Time) (int, error) {
	log.Printf("Validating interval data for %s from %v to %v", interval, startTime, endTime)

	// Align startTime and endTime to the interval boundaries
	alignedStartTime := alignToInterval(startTime, intervalConfigs[interval])
	alignedEndTime := alignToInterval(endTime, intervalConfigs[interval])

	var count int
	err := db.QueryRow(`
        SELECT COUNT(*)
        FROM btc_price_data
        WHERE interval = ? AND timestamp >= ? AND timestamp < ? AND fetched = 1
    `, interval, alignedStartTime.UnixMilli(), alignedEndTime.UnixMilli()).Scan(&count)
	if err != nil {
		log.Printf("Error querying kline count: %v", err)
		return 0, err
	}

	expectedCount := int(alignedEndTime.Sub(alignedStartTime) / intervalConfigs[interval])
	log.Printf("Found %d klines for %s, expected %d", count, interval, expectedCount)
	if count != expectedCount {
		return count, fmt.Errorf("expected %d klines, found %d", expectedCount, count)
	}

	return count, nil
}

func main() {
	log.Println("Starting BTC data fetcher application")
	db, err := initDatabase()
	if err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}
	defer db.Close()

	config := Config{
		WeeksToFetch: 12,
		RateLimit:    time.Millisecond * 100,
	}
	log.Printf("Configuration: WeeksToFetch=%d, RateLimit=%v", config.WeeksToFetch, config.RateLimit)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	g, errChan := gronos.New[string](ctx, map[string]gronos.RuntimeApplication{
		"btc_data_fetcher": CreateRunner(db, config),
	})

	go func() {
		for err := range errChan {
			log.Printf("Error from gronos: %v", err)
		}
	}()

	log.Println("BTC data fetcher is running. Press Ctrl+C to stop.")
	g.Wait()

	log.Println("BTC data fetching completed.")
}
