package main

import (
	"context"
	"log"
	"os"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"matchbox/internal/marketdata"
	"matchbox/internal/repository"
)

func main() {
	// Database Connection
	// You should set this environment variable (e.g., export DATABASE_URL="postgres://user:pass@localhost:5432/matchbox?sslmode=disable")
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		dbURL = "postgres://postgres:postgres@localhost:5432/matchbox?sslmode=disable"
	}

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		log.Fatalf("Unable to connect to database: %v\n", err)
	}
	defer pool.Close()

	log.Println("Connected to PostgreSQL (TimescaleDB)")

	klineSvc := marketdata.NewKlineService()
	repo := repository.NewKlineRepo(pool)

	symbols := []string{"BTC/USDT", "ETH/USDT", "XRP/USDT"}
	intervals := []string{"1m", "15m", "1h", "1d"}

	log.Println("Starting backfill migration for klines...")

	for _, symbol := range symbols {
		for _, interval := range intervals {
			log.Printf("Fetching 500 candles for %s (%s)...", symbol, interval)
			
			// Fetch from Binance
			fetchCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
			klines, err := klineSvc.FetchKlines(fetchCtx, symbol, interval)
			cancel()
			
			if err != nil {
				log.Printf("Failed to fetch %s (%s): %v", symbol, interval, err)
				continue
			}

			// Batch insert via COPY protocol
			insertCtx, insertCancel := context.WithTimeout(ctx, 10*time.Second)
			err = repo.BatchInsertKlines(insertCtx, symbol, interval, klines)
			insertCancel()

			if err != nil {
				log.Printf("Failed to batch insert %s (%s): %v", symbol, interval, err)
				continue
			}

			log.Printf("Successfully inserted %d candles for %s (%s)", len(klines), symbol, interval)
			
			// Sleep briefly to avoid Binance API rate limits
			time.Sleep(500 * time.Millisecond)
		}
	}

	log.Println("Kline backfill migration completed successfully!")
}
