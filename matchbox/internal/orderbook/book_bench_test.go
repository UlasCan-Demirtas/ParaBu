package orderbook

import (
	"math/rand"
	"testing"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"matchbox/pkg/types"
)

// BenchmarkLimitOrderMatching benchmarks the matching engine's throughput when matching
// incoming limit orders against a pre-filled order book of 10,000 orders.
func BenchmarkLimitOrderMatching(b *testing.B) {
	ob := NewOrderBook("BTC/USDT")

	// Pre-fill the book with 10,000 Asks
	for i := 0; i < 10000; i++ {
		price := decimal.NewFromInt(int64(60000 + i)) // 60000, 60001, ...
		qty := decimal.NewFromInt(1)
		ask := types.NewOrder(uuid.New().String(), "BTC/USDT", types.SideSell, types.OrderTypeLimit, price, qty)
		ob.ProcessLimitOrder(ask)
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		// Incoming Buy order that exactly matches the best Ask
		price := decimal.NewFromInt(int64(60000 + (i % 10000)))
		qty := decimal.NewFromInt(1)
		
		buy := types.NewOrder("user-bench", "BTC/USDT", types.SideBuy, types.OrderTypeLimit, price, qty)
		ob.ProcessLimitOrder(buy)
		
		// In a real scenario, we might release the order here if it's fully filled and done broadcasting.
		// types.ReleaseOrder(buy) 
	}
}

// BenchmarkOrderCancellation tests the O(1) order cancellation mechanism
// against a large order book filled with 100,000 orders.
func BenchmarkOrderCancellation(b *testing.B) {
	ob := NewOrderBook("ETH/USDT")
	
	orderIDs := make([]uuid.UUID, 100000)

	// Pre-fill 100,000 Bids
	for i := 0; i < 100000; i++ {
		price := decimal.NewFromInt(int64(3000 + (i % 1000))) // Randomish prices
		qty := decimal.NewFromInt(1)
		bid := types.NewOrder(uuid.New().String(), "ETH/USDT", types.SideBuy, types.OrderTypeLimit, price, qty)
		ob.ProcessLimitOrder(bid)
		orderIDs[i] = bid.ID
	}

	// Shuffle the IDs so cancellations are truly random
	rand.Shuffle(len(orderIDs), func(i, j int) {
		orderIDs[i], orderIDs[j] = orderIDs[j], orderIDs[i]
	})

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		// Just take the next ID to cancel. Wrap around if b.N > 100000
		targetID := orderIDs[i%100000]
		ob.CancelOrder(targetID)
	}
}

// BenchmarkOrderPool tests the allocation difference when creating orders.
func BenchmarkOrderCreation_WithPool(b *testing.B) {
	b.ReportAllocs()
	price := decimal.NewFromInt(60000)
	qty := decimal.NewFromInt(1)

	for i := 0; i < b.N; i++ {
		order := types.NewOrder("user-1", "BTC/USDT", types.SideBuy, types.OrderTypeLimit, price, qty)
		// Release immediately to simulate zero-allocation reuse
		types.ReleaseOrder(order)
	}
}

func BenchmarkOrderCreation_WithoutPool(b *testing.B) {
	b.ReportAllocs()
	price := decimal.NewFromInt(60000)
	qty := decimal.NewFromInt(1)

	for i := 0; i < b.N; i++ {
		_ = &types.Order{
			ID:           uuid.New(),
			UserID:       "user-1",
			Symbol:       "BTC/USDT",
			Side:         types.SideBuy,
			Type:         types.OrderTypeLimit,
			Price:        price,
			Quantity:     qty,
			RemainingQty: qty,
			Status:       types.OrderStatusNew,
		}
	}
}
