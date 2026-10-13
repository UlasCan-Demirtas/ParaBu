package marketdata

import (
	"context"
	"encoding/json"
	"log"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// BinanceTicker represents a single element from the !ticker@arr stream.
type BinanceTicker struct {
	Symbol             string `json:"s"` // Symbol
	PriceChangePercent string `json:"P"` // Price change percent
	LastPrice          string `json:"c"` // Last price
	Volume             string `json:"v"` // Total traded base asset volume
}

// TickerData represents the cleaned data we broadcast to clients.
type TickerData struct {
	Symbol             string `json:"symbol"`
	LastPrice          string `json:"lastPrice"`
	PriceChangePercent string `json:"priceChangePercent"`
	Volume             string `json:"volume"`
}

type TickerCache struct {
	mu      sync.RWMutex
	tickers map[string]TickerData
	dirty   map[string]bool // Tracks symbols that have updated
}

type TickerWorker struct {
	cache *TickerCache
	url   string
}

func NewTickerWorker() *TickerWorker {
	return &TickerWorker{
		cache: &TickerCache{
			tickers: make(map[string]TickerData),
			dirty:   make(map[string]bool),
		},
		url: "wss://stream.binance.com:9443/ws/!ticker@arr",
	}
}

// Start initiates the background loops.
func (tw *TickerWorker) Start(ctx context.Context, broadcastFn func(topic string, payload interface{})) {
	go tw.readLoop(ctx)
	go tw.publishLoop(ctx, broadcastFn)
}

func (tw *TickerWorker) readLoop(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		conn, _, err := websocket.DefaultDialer.DialContext(ctx, tw.url, nil)
		if err != nil {
			log.Printf("TickerWorker ws dial error: %v, retrying in 5s...", err)
			time.Sleep(5 * time.Second)
			continue
		}

		log.Println("TickerWorker connected to Binance !ticker@arr")

		for {
			_, message, err := conn.ReadMessage()
			if err != nil {
				log.Printf("TickerWorker read error: %v. Reconnecting...", err)
				conn.Close()
				break
			}

			var bTickers []BinanceTicker
			if err := json.Unmarshal(message, &bTickers); err != nil {
				continue
			}

			tw.cache.mu.Lock()
			for _, bt := range bTickers {
				tw.cache.tickers[bt.Symbol] = TickerData{
					Symbol:             bt.Symbol,
					LastPrice:          bt.LastPrice,
					PriceChangePercent: bt.PriceChangePercent,
					Volume:             bt.Volume,
				}
				tw.cache.dirty[bt.Symbol] = true
			}
			tw.cache.mu.Unlock()
		}
	}
}

func (tw *TickerWorker) publishLoop(ctx context.Context, broadcastFn func(topic string, payload interface{})) {
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			tw.cache.mu.Lock()
			if len(tw.cache.dirty) == 0 {
				tw.cache.mu.Unlock()
				continue
			}

			diff := make([]TickerData, 0, len(tw.cache.dirty))
			for sym := range tw.cache.dirty {
				diff = append(diff, tw.cache.tickers[sym])
			}
			
			// Clear the dirty map for the next cycle
			tw.cache.dirty = make(map[string]bool)
			tw.cache.mu.Unlock()

			// Broadcast diff
			broadcastFn("tickers", diff)
		}
	}
}
