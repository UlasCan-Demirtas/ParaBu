package marketdata

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

type PriceLevel struct {
	Price float64
	Qty   float64
}

// OrderBookEngine maintains an L2 orderbook using binary search supported slices.
// Slice copying is cache-friendly and outperforms pointer-based RB-Trees in Go for N < 5000.
type OrderBookEngine struct {
	mu           sync.RWMutex
	Symbol       string
	Bids         []PriceLevel // Sorted Descending (highest price first)
	Asks         []PriceLevel // Sorted Ascending (lowest price first)
	LastUpdateID int64
	Synced       bool
}

func NewOrderBookEngine(symbol string) *OrderBookEngine {
	return &OrderBookEngine{
		Symbol: symbol,
		Bids:   make([]PriceLevel, 0, 5000),
		Asks:   make([]PriceLevel, 0, 5000),
	}
}

// UpdateBid applies a delta to the Bids slice.
// If qty == 0, the level is deleted.
func (obe *OrderBookEngine) UpdateBid(price, qty float64) {
	idx := sort.Search(len(obe.Bids), func(i int) bool {
		return obe.Bids[i].Price <= price
	})

	if idx < len(obe.Bids) && obe.Bids[idx].Price == price {
		if qty == 0 {
			obe.Bids = append(obe.Bids[:idx], obe.Bids[idx+1:]...)
		} else {
			obe.Bids[idx].Qty = qty
		}
	} else if qty > 0 {
		obe.Bids = append(obe.Bids, PriceLevel{})
		copy(obe.Bids[idx+1:], obe.Bids[idx:])
		obe.Bids[idx] = PriceLevel{Price: price, Qty: qty}
	}
}

// UpdateAsk applies a delta to the Asks slice.
// If qty == 0, the level is deleted.
func (obe *OrderBookEngine) UpdateAsk(price, qty float64) {
	idx := sort.Search(len(obe.Asks), func(i int) bool {
		return obe.Asks[i].Price >= price
	})

	if idx < len(obe.Asks) && obe.Asks[idx].Price == price {
		if qty == 0 {
			obe.Asks = append(obe.Asks[:idx], obe.Asks[idx+1:]...)
		} else {
			obe.Asks[idx].Qty = qty
		}
	} else if qty > 0 {
		obe.Asks = append(obe.Asks, PriceLevel{})
		copy(obe.Asks[idx+1:], obe.Asks[idx:])
		obe.Asks[idx] = PriceLevel{Price: price, Qty: qty}
	}
}

type DepthEvent struct {
	Event         string     `json:"e"`
	EventTime     int64      `json:"E"`
	Symbol        string     `json:"s"`
	FirstUpdateID int64      `json:"U"`
	FinalUpdateID int64      `json:"u"`
	Bids          [][]string `json:"b"`
	Asks          [][]string `json:"a"`
}

type DepthSnapshot struct {
	LastUpdateId int64      `json:"lastUpdateId"`
	Bids         [][]string `json:"bids"`
	Asks         [][]string `json:"asks"`
}

// L2SyncWorker synchronizes a local OrderBookEngine with Binance.
type L2SyncWorker struct {
	OBE    *OrderBookEngine
	Symbol string
}

func NewL2SyncWorker(symbol string) *L2SyncWorker {
	return &L2SyncWorker{
		OBE:    NewOrderBookEngine(symbol),
		Symbol: symbol,
	}
}

// Start initiates the snapshot retrieval and delta stream buffering.
func (w *L2SyncWorker) Start(ctx context.Context) {
	go w.run(ctx)
}

func (w *L2SyncWorker) run(ctx context.Context) {
	// e.g., "BTCUSDT" -> "btcusdt"
	lowerSymbol := strings.ToLower(strings.ReplaceAll(w.Symbol, "/", ""))
	upperSymbol := strings.ToUpper(strings.ReplaceAll(w.Symbol, "/", ""))

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		wsURL := fmt.Sprintf("wss://stream.binance.com:9443/ws/%s@depth", lowerSymbol)
		conn, _, err := websocket.DefaultDialer.DialContext(ctx, wsURL, nil)
		if err != nil {
			log.Printf("L2SyncWorker WS dial error for %s: %v, retrying...", w.Symbol, err)
			time.Sleep(2 * time.Second)
			continue
		}

		buffer := make(chan DepthEvent, 1000)
		readErrCh := make(chan error, 1)

		// 1. Concurrently buffer WS events
		go func() {
			defer close(buffer)
			for {
				_, msg, err := conn.ReadMessage()
				if err != nil {
					readErrCh <- err
					return
				}
				var event DepthEvent
				if err := json.Unmarshal(msg, &event); err == nil {
					buffer <- event
				}
			}
		}()

		// 2. Fetch REST Snapshot
		snapshotURL := fmt.Sprintf("https://api.binance.com/api/v3/depth?symbol=%s&limit=1000", upperSymbol)
		req, _ := http.NewRequestWithContext(ctx, "GET", snapshotURL, nil)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			conn.Close()
			time.Sleep(time.Second)
			continue
		}

		var snap DepthSnapshot
		if err := json.NewDecoder(resp.Body).Decode(&snap); err != nil {
			resp.Body.Close()
			conn.Close()
			time.Sleep(time.Second)
			continue
		}
		resp.Body.Close()

		// 3. Initialize OBE with snapshot
		w.OBE.mu.Lock()
		w.OBE.Bids = make([]PriceLevel, 0, 5000)
		w.OBE.Asks = make([]PriceLevel, 0, 5000)
		w.OBE.LastUpdateID = snap.LastUpdateId
		
		for _, b := range snap.Bids {
			p, _ := strconv.ParseFloat(b[0], 64)
			q, _ := strconv.ParseFloat(b[1], 64)
			w.OBE.UpdateBid(p, q)
		}
		for _, a := range snap.Asks {
			p, _ := strconv.ParseFloat(a[0], 64)
			q, _ := strconv.ParseFloat(a[1], 64)
			w.OBE.UpdateAsk(p, q)
		}
		w.OBE.Synced = false
		w.OBE.mu.Unlock()

		log.Printf("[%s] OrderBook snapshot initialized. LastUpdateID: %d", w.Symbol, snap.LastUpdateId)

		var previousU int64 = 0
		reconnectRequired := false

		// 4. Process buffered deltas
	ProcessLoop:
		for {
			select {
			case <-ctx.Done():
				conn.Close()
				return
			case err := <-readErrCh:
				log.Printf("[%s] OrderBook stream broken: %v", w.Symbol, err)
				reconnectRequired = true
				break ProcessLoop
			case event, ok := <-buffer:
				if !ok {
					reconnectRequired = true
					break ProcessLoop
				}

				if !w.OBE.Synced {
					// Drop older events
					if event.FinalUpdateID <= snap.LastUpdateId {
						continue
					}
					// First valid event must span across the snapshot's LastUpdateId
					if event.FirstUpdateID <= snap.LastUpdateId+1 && event.FinalUpdateID >= snap.LastUpdateId+1 {
						w.OBE.Synced = true
					} else {
						log.Printf("[%s] Sync missed. Reconnecting...", w.Symbol)
						reconnectRequired = true
						break ProcessLoop
					}
				} else {
					// Enforce sequence constraint
					if event.FirstUpdateID != previousU+1 {
						log.Printf("[%s] Sequence gap detected (Expected: %d, Got: %d). Reconnecting...", w.Symbol, previousU+1, event.FirstUpdateID)
						reconnectRequired = true
						break ProcessLoop
					}
				}

				previousU = event.FinalUpdateID

				w.OBE.mu.Lock()
				for _, b := range event.Bids {
					p, _ := strconv.ParseFloat(b[0], 64)
					q, _ := strconv.ParseFloat(b[1], 64)
					w.OBE.UpdateBid(p, q)
				}
				for _, a := range event.Asks {
					p, _ := strconv.ParseFloat(a[0], 64)
					q, _ := strconv.ParseFloat(a[1], 64)
					w.OBE.UpdateAsk(p, q)
				}
				w.OBE.LastUpdateID = event.FinalUpdateID
				w.OBE.mu.Unlock()
			}
		}

		conn.Close()
		if reconnectRequired {
			time.Sleep(1 * time.Second)
		}
	}
}

// GetTopN returns a safe copy of the top N bids and asks for broadcasting
func (obe *OrderBookEngine) GetTopN(n int) ([]PriceLevel, []PriceLevel) {
	obe.mu.RLock()
	defer obe.mu.RUnlock()

	bidsCount := n
	if len(obe.Bids) < n {
		bidsCount = len(obe.Bids)
	}
	asksCount := n
	if len(obe.Asks) < n {
		asksCount = len(obe.Asks)
	}

	bids := make([]PriceLevel, bidsCount)
	copy(bids, obe.Bids[:bidsCount])

	asks := make([]PriceLevel, asksCount)
	copy(asks, obe.Asks[:asksCount])

	return bids, asks
}
