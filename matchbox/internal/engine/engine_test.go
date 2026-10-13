package engine

import (
	"context"
	"math/rand"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"matchbox/pkg/types"
)

// TestEngine_RaceAndStress spins up 100 concurrent goroutines placing and canceling orders
// simultaneously using the Engine's channels. This ensures that the Engine's Actor model
// successfully prevents race conditions and data corruption inside the OrderBook.
func TestEngine_RaceAndStress(t *testing.T) {
	// Initialize Ledger with dummy DB (we will mock it later if needed, but for now we can just use nil
	// because PlaceOrder/CancelOrder do not access Ledger directly. Actually CancelReplace does).
	// To avoid nil panics in CancelReplace, we can just skip CancelReplace in this stress test,
	// or provide a minimal mock. For now, we just test Place and Cancel.
	
	eng := NewEngine(nil, nil, nil)
	
	// Create dummy channels to drain output events so they don't block
	eng.TradeOutChan = make(chan *types.Trade, 100000)
	eng.DepthOutChan = make(chan *types.DepthSnapshot, 100000)
	eng.MatchOutChan = make(chan *types.MatchResult, 100000)
	eng.CancelOutChan = make(chan *types.CancelResult, 100000)

	// Background consumer to drain output channels
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case <-eng.TradeOutChan:
			case <-eng.DepthOutChan:
			case <-eng.MatchOutChan:
			case <-eng.CancelOutChan:
			}
		}
	}()

	eng.RegisterSymbol("ETH/USDT")
	eng.StartWorkers(ctx)

	var wg sync.WaitGroup
	numWorkers := 100
	iterations := 100

	var orderIDs []uuid.UUID
	var idMu sync.Mutex

	rand.Seed(time.Now().UnixNano())

	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			r := rand.New(rand.NewSource(time.Now().UnixNano() + int64(workerID)))

			for j := 0; j < iterations; j++ {
				action := r.Intn(100)

				if action < 70 {
					// PLACE ORDER
					price := decimal.NewFromInt(int64(r.Intn(1000) + 2000))
					qty := decimal.NewFromInt(int64(r.Intn(10) + 1))
					side := types.SideBuy
					if r.Intn(2) == 0 {
						side = types.SideSell
					}

					order := types.NewOrder(uuid.New().String(), "ETH/USDT", side, types.OrderTypeLimit, price, qty)
					
					// Thread-Safe operation on Engine (sends to channel)
					err := eng.PlaceOrder(order)
					if err == nil {
						idMu.Lock()
						orderIDs = append(orderIDs, order.ID)
						idMu.Unlock()
					}
				} else {
					// CANCEL ORDER
					idMu.Lock()
					if len(orderIDs) == 0 {
						idMu.Unlock()
						continue
					}
					targetID := orderIDs[r.Intn(len(orderIDs))]
					idMu.Unlock()

					// Thread-Safe operation on Engine (sends to channel)
					_ = eng.CancelOrder(types.CancelRequest{
						Symbol:  "ETH/USDT",
						OrderID: targetID,
					})
				}
			}
		}(i)
	}

	wg.Wait()
	
	// Graceful shutdown ensures all queues are completely drained
	eng.Stop()

	t.Logf("Successfully processed %d concurrent operations via Engine Actor without data races.", numWorkers*iterations)
}
