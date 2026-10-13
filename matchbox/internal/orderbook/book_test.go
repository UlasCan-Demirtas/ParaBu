package orderbook

import (
	"testing"
	"github.com/shopspring/decimal"
	"matchbox/pkg/types"
)

func TestOrderBook_ProcessLimitOrder(t *testing.T) {
	ob := NewOrderBook("BTC/USDT")

	// Helper to create decimals easily
	d := func(s string) decimal.Decimal {
		dec, _ := decimal.NewFromString(s)
		return dec
	}

	// 1. Add some asks (sell orders)
	ask1 := types.NewOrder("user1", "BTC/USDT", types.SideSell, types.OrderTypeLimit, d("60000"), d("1.5"))
	ask2 := types.NewOrder("user1", "BTC/USDT", types.SideSell, types.OrderTypeLimit, d("60100"), d("2.0"))

	ob.ProcessLimitOrder(ask1)
	ob.ProcessLimitOrder(ask2)

	// Check book state
	if ob.asks.Len() != 2 {
		t.Fatalf("Expected 2 asks, got %d", ob.asks.Len())
	}
	if ob.bids.Len() != 0 {
		t.Fatalf("Expected 0 bids, got %d", ob.bids.Len())
	}

	// 2. Add a buy order that partially fills against the best ask
	buy1 := types.NewOrder("user1", "BTC/USDT", types.SideBuy, types.OrderTypeLimit, d("60000"), d("1.0"))
	res := ob.ProcessLimitOrder(buy1)

	if len(res.Trades) != 1 {
		t.Fatalf("Expected 1 trade, got %d", len(res.Trades))
	}
	trade := res.Trades[0]
	if !trade.Price.Equal(d("60000")) || !trade.Quantity.Equal(d("1.0")) {
		t.Errorf("Trade incorrect: expected 1.0 @ 60000, got %s @ %s", trade.Quantity.String(), trade.Price.String())
	}

	if buy1.Status != types.OrderStatusFilled {
		t.Errorf("Expected buy1 to be filled, got %s", buy1.Status)
	}
	if ask1.Status != types.OrderStatusPartiallyFilled {
		t.Errorf("Expected ask1 to be partially filled, got %s", ask1.Status)
	}
	if !ask1.RemainingQty.Equal(d("0.5")) {
		t.Errorf("Expected ask1 remaining to be 0.5, got %s", ask1.RemainingQty.String())
	}

	// 3. Add a buy order that sweeps the rest of ask1 and partially fills ask2
	buy2 := types.NewOrder("user1", "BTC/USDT", types.SideBuy, types.OrderTypeLimit, d("60100"), d("1.5"))
	res2 := ob.ProcessLimitOrder(buy2)

	if len(res2.Trades) != 2 {
		t.Fatalf("Expected 2 trades, got %d", len(res2.Trades))
	}

	if !res2.Trades[0].Quantity.Equal(d("0.5")) || !res2.Trades[0].Price.Equal(d("60000")) {
		t.Errorf("First trade incorrect")
	}
	if !res2.Trades[1].Quantity.Equal(d("1.0")) || !res2.Trades[1].Price.Equal(d("60100")) {
		t.Errorf("Second trade incorrect")
	}

	if ask1.Status != types.OrderStatusFilled {
		t.Errorf("Expected ask1 to be filled")
	}
	if ask2.Status != types.OrderStatusPartiallyFilled {
		t.Errorf("Expected ask2 to be partially filled")
	}

	// Check B-Tree cleanup
	if ob.asks.Len() != 1 {
		t.Errorf("Expected 1 price level in asks tree (ask1 should be removed), got %d", ob.asks.Len())
	}
}

func TestOrderBook_CancelOrder(t *testing.T) {
	ob := NewOrderBook("BTC/USDT")
	d := func(s string) decimal.Decimal { dec, _ := decimal.NewFromString(s); return dec }

	order := types.NewOrder("user1", "BTC/USDT", types.SideBuy, types.OrderTypeLimit, d("50000"), d("1.0"))
	ob.ProcessLimitOrder(order)

	if ob.bids.Len() != 1 {
		t.Fatalf("Expected 1 bid")
	}

	res := ob.CancelOrder(order.ID)
	if !res.Success {
		t.Fatalf("Cancel failed")
	}

	if ob.bids.Len() != 0 {
		t.Fatalf("Expected bid to be removed from tree")
	}

	if order.Status != types.OrderStatusCancelled {
		t.Errorf("Expected status CANCELLED, got %s", order.Status)
	}

	// Double cancel should fail gracefully
	res2 := ob.CancelOrder(order.ID)
	if res2.Success {
		t.Fatalf("Second cancel should fail")
	}
}

func TestOrderBook_PriceTimePriority(t *testing.T) {
	ob := NewOrderBook("BTC/USDT")
	d := func(s string) decimal.Decimal { dec, _ := decimal.NewFromString(s); return dec }

	// Two asks at same price
	ask1 := types.NewOrder("user1", "BTC/USDT", types.SideSell, types.OrderTypeLimit, d("60000"), d("1.0"))
	ask2 := types.NewOrder("user1", "BTC/USDT", types.SideSell, types.OrderTypeLimit, d("60000"), d("1.0"))

	ob.ProcessLimitOrder(ask1)
	ob.ProcessLimitOrder(ask2)

	// Buy 1.0 at 60000. It should match with ask1 because it arrived first
	buy := types.NewOrder("user1", "BTC/USDT", types.SideBuy, types.OrderTypeLimit, d("60000"), d("1.0"))
	res := ob.ProcessLimitOrder(buy)

	if len(res.Trades) != 1 {
		t.Fatalf("Expected 1 trade")
	}

	trade := res.Trades[0]
	if trade.SellOrderID != ask1.ID {
		t.Errorf("Expected trade to match with first ask (ask1), but matched with %s", trade.SellOrderID)
	}
}

func TestOrderBook_MarketOrder(t *testing.T) {
	ob := NewOrderBook("BTC/USDT")
	d := func(s string) decimal.Decimal { dec, _ := decimal.NewFromString(s); return dec }

	ask1 := types.NewOrder("user1", "BTC/USDT", types.SideSell, types.OrderTypeLimit, d("60000"), d("1.0"))
	ob.ProcessLimitOrder(ask1)

	// Market order for 2.0 (more than available)
	marketBuy := types.NewOrder("user1", "BTC/USDT", types.SideBuy, types.OrderTypeMarket, decimal.Zero, d("2.0"))
	res := ob.ProcessMarketOrder(marketBuy)

	if len(res.Trades) != 1 {
		t.Fatalf("Expected 1 trade")
	}

	if !res.Trades[0].Quantity.Equal(d("1.0")) {
		t.Errorf("Expected 1.0 traded, got %s", res.Trades[0].Quantity.String())
	}

	if marketBuy.Status != types.OrderStatusCancelled {
		t.Errorf("Expected remaining market order to be cancelled, got %s", marketBuy.Status)
	}

	if ob.bids.Len() != 0 {
		t.Errorf("Market order should not be added to the book")
	}
}

// FuzzProcessLimitOrder tests the matching engine against random, extreme, and negative values.
func FuzzProcessLimitOrder(f *testing.F) {
	// Seed corpus with some basic scenarios
	f.Add(float64(1.5), float64(100.0), true)   // Normal Buy
	f.Add(float64(1.5), float64(100.0), false)  // Normal Sell
	f.Add(float64(-1.0), float64(-50.0), true)  // Negative Price/Qty
	f.Add(float64(0.0), float64(0.0), false)    // Zero Price/Qty
	f.Add(float64(1e12), float64(1e12), true)   // Large Numbers

	f.Fuzz(func(t *testing.T, priceFloat, qtyFloat float64, isBuy bool) {
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("Engine panicked with price=%f, qty=%f, isBuy=%v. Panic: %v", priceFloat, qtyFloat, isBuy, r)
			}
		}()

		ob := NewOrderBook("BTC/USDT")
		
		price := decimal.NewFromFloat(priceFloat)
		qty := decimal.NewFromFloat(qtyFloat)
		
		side := types.SideBuy
		if !isBuy {
			side = types.SideSell
		}

		order := types.NewOrder("user-fuzz", "BTC/USDT", side, types.OrderTypeLimit, price, qty)
		
		matchRes := ob.ProcessLimitOrder(order)
		if matchRes == nil {
			t.Fatal("ProcessLimitOrder returned nil")
		}

		ob.CancelOrder(order.ID)

		ob.CancelReplace(types.CancelReplaceRequest{
			OriginalOrderID: order.ID,
			Symbol:          "BTC/USDT",
			NewPrice:        price.Add(decimal.NewFromInt(1)),
			NewQuantity:     qty.Add(decimal.NewFromInt(1)),
		}, func(oldOrder *types.Order) error {
			return nil
		})
	})
}


