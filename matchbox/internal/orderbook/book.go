package orderbook

import (
	"container/list"
	"time"

	"github.com/google/btree"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"matchbox/pkg/types"
)

// PriceLevel represents a single price level in the order book.
// It maintains a queue of orders (FIFO) at this price.
type PriceLevel struct {
	Price  decimal.Decimal
	Orders *list.List // Queue of *types.Order
	Volume decimal.Decimal
}

// NewPriceLevel creates a new price level.
func NewPriceLevel(price decimal.Decimal) *PriceLevel {
	return &PriceLevel{
		Price:  price,
		Orders: list.New(),
		Volume: decimal.Zero,
	}
}

// AddOrder appends an order to the end of the queue (FIFO).
func (pl *PriceLevel) AddOrder(order *types.Order) *list.Element {
	pl.Volume = pl.Volume.Add(order.RemainingQty)
	return pl.Orders.PushBack(order)
}

// RemoveOrder removes an order from the queue and updates volume.
func (pl *PriceLevel) RemoveOrder(element *list.Element) {
	if element != nil {
		order := element.Value.(*types.Order)
		pl.Volume = pl.Volume.Sub(order.RemainingQty)
		pl.Orders.Remove(element)
	}
}

// BTree wrappers to implement btree.Item interface
type BidItem struct {
	*PriceLevel
}

func (a BidItem) Less(then btree.Item) bool {
	// Bids are sorted descending (highest price first)
	// So a < b means a.Price > b.Price
	return a.Price.GreaterThan(then.(BidItem).Price)
}

type AskItem struct {
	*PriceLevel
}

func (a AskItem) Less(then btree.Item) bool {
	// Asks are sorted ascending (lowest price first)
	// So a < b means a.Price < b.Price
	return a.Price.LessThan(then.(AskItem).Price)
}

// OrderBook represents a single market's order book (e.g., BTC/USDT).
type OrderBook struct {
	Symbol string

	// B-Trees for fast O(log N) price level lookups and ordered iteration
	bids *btree.BTree
	asks *btree.BTree

	// O(1) lookup maps for quick cancellation and status checks
	orderIndex map[uuid.UUID]*list.Element
	priceIndex map[string]*PriceLevel
}

// NewOrderBook creates a new order book instance.
func NewOrderBook(symbol string) *OrderBook {
	return &OrderBook{
		Symbol:     symbol,
		bids:       btree.New(32), // Degree 32 is a reasonable default for memory/performance
		asks:       btree.New(32),
		orderIndex: make(map[uuid.UUID]*list.Element),
		priceIndex: make(map[string]*PriceLevel),
	}
}

// ExportOrders extracts all resting orders in the book for snapshotting.
func (ob *OrderBook) ExportOrders() []*types.Order {
	var orders []*types.Order
	
	// Collect Asks
	ob.asks.Ascend(func(i btree.Item) bool {
		pl := i.(AskItem).PriceLevel
		for e := pl.Orders.Front(); e != nil; e = e.Next() {
			orders = append(orders, e.Value.(*types.Order))
		}
		return true
	})

	// Collect Bids
	ob.bids.Ascend(func(i btree.Item) bool {
		pl := i.(BidItem).PriceLevel
		for e := pl.Orders.Front(); e != nil; e = e.Next() {
			orders = append(orders, e.Value.(*types.Order))
		}
		return true
	})

	return orders
}

// RestoreOrder directly places an order into the book without matching.
// This is used ONLY by the RecoveryEngine when loading from a snapshot.
func (ob *OrderBook) RestoreOrder(order *types.Order) {
	ob.addOrder(order)
}

// ProcessLimitOrder attempts to match a limit order and adds the remainder to the book.
func (ob *OrderBook) ProcessLimitOrder(order *types.Order) *types.MatchResult {
	result := &types.MatchResult{
		TakerOrder: order,
		Trades:     make([]*types.Trade, 0),
	}

	// 1. Match against opposite side
	ob.matchOrder(order, result)

	// 2. If there's still quantity remaining, add it to the book
	if !order.IsFilled() && order.Status != types.OrderStatusCancelled {
		ob.addOrder(order)
	}

	return result
}

// ProcessMarketOrder attempts to match a market order immediately. Remainder is cancelled.
func (ob *OrderBook) ProcessMarketOrder(order *types.Order) *types.MatchResult {
	result := &types.MatchResult{
		TakerOrder: order,
		Trades:     make([]*types.Trade, 0),
	}

	// Match against opposite side
	ob.matchOrder(order, result)

	// Market orders don't rest on the book. If not fully filled, cancel the remainder.
	if !order.IsFilled() {
		order.Status = types.OrderStatusCancelled
	}

	return result
}

// matchOrder executes the matching logic against the contra side.
func (ob *OrderBook) matchOrder(takerOrder *types.Order, result *types.MatchResult) {
	var contraTree *btree.BTree

	if takerOrder.Side == types.SideBuy {
		contraTree = ob.asks
	} else {
		contraTree = ob.bids
	}

	levelsToRemove := make([]btree.Item, 0)

	// Iterate through the contra side starting from the best price
	contraTree.Ascend(func(i btree.Item) bool {
		if takerOrder.IsFilled() {
			return false // Stop iterating if taker is filled
		}

		var priceLevel *PriceLevel
		if takerOrder.Side == types.SideBuy {
			priceLevel = i.(AskItem).PriceLevel
			// Limit order protection: if best ask > taker buy limit price, stop matching
			if takerOrder.Type == types.OrderTypeLimit && priceLevel.Price.GreaterThan(takerOrder.Price) {
				return false
			}
		} else {
			priceLevel = i.(BidItem).PriceLevel
			// Limit order protection: if best bid < taker sell limit price, stop matching
			if takerOrder.Type == types.OrderTypeLimit && priceLevel.Price.LessThan(takerOrder.Price) {
				return false
			}
		}

		// Iterate through orders at this price level (FIFO)
		element := priceLevel.Orders.Front()
		for element != nil {
			if takerOrder.IsFilled() {
				break
			}

			makerOrder := element.Value.(*types.Order)

			// Determine trade quantity (min of taker remaining and maker remaining)
			tradeQty := decimal.Min(takerOrder.RemainingQty, makerOrder.RemainingQty)

			// Create trade
			var trade *types.Trade
			if takerOrder.Side == types.SideBuy {
				trade = types.NewTrade(ob.Symbol, takerOrder.ID, takerOrder.UserID, takerOrder.HoldID, makerOrder.ID, makerOrder.UserID, makerOrder.HoldID, makerOrder.Price, tradeQty)
			} else {
				trade = types.NewTrade(ob.Symbol, makerOrder.ID, makerOrder.UserID, makerOrder.HoldID, takerOrder.ID, takerOrder.UserID, takerOrder.HoldID, makerOrder.Price, tradeQty)
			}
			result.Trades = append(result.Trades, trade)

			// Update orders
			takerOrder.RemainingQty = takerOrder.RemainingQty.Sub(tradeQty)
			makerOrder.RemainingQty = makerOrder.RemainingQty.Sub(tradeQty)
			priceLevel.Volume = priceLevel.Volume.Sub(tradeQty)

			// Update maker status
			if makerOrder.IsFilled() {
				makerOrder.Status = types.OrderStatusFilled

				// Remove maker from index
				delete(ob.orderIndex, makerOrder.ID)

				// Remove from price level queue
				next := element.Next()
				priceLevel.Orders.Remove(element)
				element = next
			} else {
				makerOrder.Status = types.OrderStatusPartiallyFilled
				element = element.Next()
			}
		}

		// If price level is empty, mark for removal
		if priceLevel.Orders.Len() == 0 {
			levelsToRemove = append(levelsToRemove, i)
		}

		return true // Continue to next price level
	})

	// Clean up empty price levels from the B-Tree
	for _, item := range levelsToRemove {
		contraTree.Delete(item)
		var price decimal.Decimal
		if takerOrder.Side == types.SideBuy {
			price = item.(AskItem).Price
		} else {
			price = item.(BidItem).Price
		}
		delete(ob.priceIndex, price.String())
	}

	// Update taker status
	if takerOrder.IsFilled() {
		takerOrder.Status = types.OrderStatusFilled
	} else if takerOrder.RemainingQty.LessThan(takerOrder.Quantity) {
		takerOrder.Status = types.OrderStatusPartiallyFilled
	}
}

// addOrder adds a resting limit order to the book.
func (ob *OrderBook) addOrder(order *types.Order) {
	// Find or create price level
	priceLevel, exists := ob.priceIndex[order.Price.String()]
	if !exists {
		priceLevel = NewPriceLevel(order.Price)
		ob.priceIndex[order.Price.String()] = priceLevel

		if order.Side == types.SideBuy {
			ob.bids.ReplaceOrInsert(BidItem{priceLevel})
		} else {
			ob.asks.ReplaceOrInsert(AskItem{priceLevel})
		}
	}

	// Add order to price level
	element := priceLevel.AddOrder(order)
	ob.orderIndex[order.ID] = element
}

// CancelOrder attempts to remove an order from the book.
func (ob *OrderBook) CancelOrder(orderID uuid.UUID) *types.CancelResult {
	element, exists := ob.orderIndex[orderID]
	if !exists {
		return &types.CancelResult{
			OrderID: orderID,
			Success: false,
			Reason:  "order not found or already executed",
		}
	}

	order := element.Value.(*types.Order)
	priceLevel := ob.priceIndex[order.Price.String()]

	// Remove from price level queue
	priceLevel.RemoveOrder(element)

	// Remove from index
	delete(ob.orderIndex, orderID)

	// Update order status
	order.Status = types.OrderStatusCancelled
	remaining := order.RemainingQty
	order.RemainingQty = decimal.Zero

	// If price level is empty, remove it from B-Tree
	if priceLevel.Orders.Len() == 0 {
		if order.Side == types.SideBuy {
			ob.bids.Delete(BidItem{priceLevel})
		} else {
			ob.asks.Delete(AskItem{priceLevel})
		}
		delete(ob.priceIndex, order.Price.String())
	}

	return &types.CancelResult{
		OrderID:   orderID,
		Success:   true,
		Remaining: remaining,
		Order:     order,
	}
}

// CancelReplace attempts to atomically cancel an old order and place a new one.
// The provided checkBalance function is executed BEFORE modifying the book.
// If checkBalance returns an error, the book remains untouched (O(1) Rollback).
func (ob *OrderBook) CancelReplace(req types.CancelReplaceRequest, checkBalance func(oldOrder *types.Order) error) *types.CancelReplaceResult {
	element, exists := ob.orderIndex[req.OriginalOrderID]
	if !exists {
		return &types.CancelReplaceResult{Success: false, Reason: "order not found or already executed"}
	}

	oldOrder := element.Value.(*types.Order)
	
	// Execute the balance check closure provided by the caller (Engine)
	err := checkBalance(oldOrder)
	if err != nil {
		// ROLLBACK: We did nothing, so the old order keeps its place in the time priority queue!
		return &types.CancelReplaceResult{Success: false, Reason: "insufficient balance: " + err.Error(), OldOrder: oldOrder}
	}

	// Balance check passed! Atomically perform the replacement.
	
	// 1. O(1) Cancel the old order
	ob.CancelOrder(req.OriginalOrderID)

	// 2. Create the new order
	newOrder := types.NewOrder(oldOrder.UserID, oldOrder.Symbol, oldOrder.Side, types.OrderTypeLimit, req.NewPrice, req.NewQuantity)
	
	// 3. Process the new order (this might match immediately or rest)
	matchRes := ob.ProcessLimitOrder(newOrder)

	return &types.CancelReplaceResult{
		Success:     true,
		OldOrder:    oldOrder,
		NewOrder:    newOrder,
		MatchResult: matchRes,
	}
}

// GetDepthSnapshot returns a snapshot of the current order book depth.
func (ob *OrderBook) GetDepthSnapshot(limit int) *types.DepthSnapshot {
	snapshot := &types.DepthSnapshot{
		Symbol:    ob.Symbol,
		Bids:      make([]types.DepthLevel, 0),
		Asks:      make([]types.DepthLevel, 0),
		Timestamp: time.Now(),
	}

	ob.bids.Ascend(func(i btree.Item) bool {
		pl := i.(BidItem).PriceLevel
		snapshot.Bids = append(snapshot.Bids, types.DepthLevel{
			Price:    pl.Price,
			Quantity: pl.Volume,
			Count:    pl.Orders.Len(),
		})
		return len(snapshot.Bids) < limit
	})

	ob.asks.Ascend(func(i btree.Item) bool {
		pl := i.(AskItem).PriceLevel
		snapshot.Asks = append(snapshot.Asks, types.DepthLevel{
			Price:    pl.Price,
			Quantity: pl.Volume,
			Count:    pl.Orders.Len(),
		})
		return len(snapshot.Asks) < limit
	})

	return snapshot
}
