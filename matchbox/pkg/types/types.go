// Package types defines the core domain types used across the matchbox matching engine.
// These types are intentionally placed in pkg/ to allow external consumers (e.g., gRPC clients,
// downstream services) to import them without pulling in internal implementation details.
package types

import (
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// Side represents the direction of an order.
type Side int

const (
	SideBuy  Side = iota // Bid side — wants to buy the base asset
	SideSell             // Ask side — wants to sell the base asset
)

func (s Side) String() string {
	switch s {
	case SideBuy:
		return "BUY"
	case SideSell:
		return "SELL"
	default:
		return "UNKNOWN"
	}
}

// Opposite returns the opposing side. Useful when scanning the contra-book for matches.
func (s Side) Opposite() Side {
	if s == SideBuy {
		return SideSell
	}
	return SideBuy
}

// OrderType distinguishes between limit and market orders.
type OrderType int

const (
	OrderTypeLimit  OrderType = iota // Limit: execute at specified price or better
	OrderTypeMarket                  // Market: execute immediately at best available price
)

func (t OrderType) String() string {
	switch t {
	case OrderTypeLimit:
		return "LIMIT"
	case OrderTypeMarket:
		return "MARKET"
	default:
		return "UNKNOWN"
	}
}

// OrderStatus tracks the lifecycle of an order.
type OrderStatus int

const (
	OrderStatusNew             OrderStatus = iota // Accepted, not yet matched
	OrderStatusPartiallyFilled                    // Some quantity filled, remainder still active
	OrderStatusFilled                             // Fully filled
	OrderStatusCancelled                          // Cancelled by user before full fill
)

func (s OrderStatus) String() string {
	switch s {
	case OrderStatusNew:
		return "NEW"
	case OrderStatusPartiallyFilled:
		return "PARTIALLY_FILLED"
	case OrderStatusFilled:
		return "FILLED"
	case OrderStatusCancelled:
		return "CANCELLED"
	default:
		return "UNKNOWN"
	}
}

// Order represents a single order submitted to the matching engine.
// Orders are immutable once created; only RemainingQty and Status are modified during matching.
type Order struct {
	ID           uuid.UUID       `json:"id"`
	UserID       string          `json:"user_id"`       // The user who placed the order
	Symbol       string          `json:"symbol"`        // e.g. "BTC/USDT"
	Side         Side            `json:"side"`          // BUY or SELL
	Type         OrderType       `json:"type"`          // LIMIT or MARKET
	Price        decimal.Decimal `json:"price"`         // Limit price; ignored for market orders
	Quantity     decimal.Decimal `json:"quantity"`      // Original requested quantity
	RemainingQty decimal.Decimal `json:"remaining_qty"` // Unfilled portion; decremented on match
	Status       OrderStatus     `json:"status"`        // Current lifecycle status
	CreatedAt    time.Time       `json:"created_at"`    // Timestamp for price-time priority (FIFO)
	HoldID       uuid.UUID       `json:"hold_id"`       // The reservation Hold ID in the double-entry ledger
}

var orderPool = sync.Pool{
	New: func() interface{} {
		return new(Order)
	},
}

// NewOrder creates a new order with a generated UUID, pooling memory to reduce allocations.
func NewOrder(userID, symbol string, side Side, orderType OrderType, price, quantity decimal.Decimal) *Order {
	o := orderPool.Get().(*Order)
	o.ID = uuid.New()
	o.UserID = userID
	o.Symbol = symbol
	o.Side = side
	o.Type = orderType
	o.Price = price
	o.Quantity = quantity
	o.RemainingQty = quantity
	o.Status = OrderStatusNew
	o.CreatedAt = time.Now()
	return o
}

// ReleaseOrder returns an Order to the pool to prevent heap allocations.
func ReleaseOrder(o *Order) {
	if o == nil {
		return
	}
	// Clear strings to prevent memory leaks
	o.UserID = ""
	o.Symbol = ""
	orderPool.Put(o)
}

// IsFilled returns true if the order has been completely filled.
func (o *Order) IsFilled() bool {
	return o.RemainingQty.IsZero()
}

// Trade represents a single fill event produced when two orders match.
type Trade struct {
	ID          uuid.UUID       `json:"id"`
	Symbol      string          `json:"symbol"`
	BuyOrderID  uuid.UUID       `json:"buy_order_id"`
	BuyUserID   string          `json:"buy_user_id"`
	BuyHoldID   uuid.UUID       `json:"buy_hold_id"`
	SellOrderID uuid.UUID       `json:"sell_order_id"`
	SellUserID  string          `json:"sell_user_id"`
	SellHoldID  uuid.UUID       `json:"sell_hold_id"`
	Price       decimal.Decimal `json:"price"`    // Execution price (maker's price)
	Quantity    decimal.Decimal `json:"quantity"` // Executed quantity
	ExecutedAt  time.Time       `json:"executed_at"`
}

// NewTrade creates a trade record from a matched buyer and seller.
func NewTrade(symbol string, buyOrderID uuid.UUID, buyUserID string, buyHoldID uuid.UUID, sellOrderID uuid.UUID, sellUserID string, sellHoldID uuid.UUID, price, quantity decimal.Decimal) *Trade {
	return &Trade{
		ID:          uuid.New(),
		Symbol:      symbol,
		BuyOrderID:  buyOrderID,
		BuyUserID:   buyUserID,
		BuyHoldID:   buyHoldID,
		SellOrderID: sellOrderID,
		SellUserID:  sellUserID,
		SellHoldID:  sellHoldID,
		Price:       price,
		Quantity:    quantity,
		ExecutedAt:  time.Now(),
	}
}

// DepthLevel represents a single price level in the order book depth chart.
type DepthLevel struct {
	Price    decimal.Decimal `json:"price"`
	Quantity decimal.Decimal `json:"quantity"` // Aggregate quantity at this price level
	Count    int             `json:"count"`    // Number of orders at this level
}

// DepthSnapshot is a point-in-time snapshot of the order book, used for WebSocket broadcasts.
type DepthSnapshot struct {
	Symbol    string       `json:"symbol"`
	Bids      []DepthLevel `json:"bids"` // Sorted descending by price (best bid first)
	Asks      []DepthLevel `json:"asks"` // Sorted ascending by price (best ask first)
	Timestamp time.Time    `json:"timestamp"`
}

// CancelRequest represents a request to cancel an existing order.
type CancelRequest struct {
	OrderID uuid.UUID `json:"order_id"`
	Symbol  string    `json:"symbol"`
}

// CancelResult represents the outcome of a cancel attempt.
type CancelResult struct {
	OrderID   uuid.UUID       `json:"order_id"`
	Success   bool            `json:"success"`
	Reason    string          `json:"reason,omitempty"` // Non-empty if success=false
	Remaining decimal.Decimal `json:"remaining"`        // Unfilled quantity at time of cancel
	Order     *Order          `json:"order,omitempty"`  // The cancelled order details
}

// CancelReplaceRequest represents an atomic cancel and replace operation.
type CancelReplaceRequest struct {
	OriginalOrderID uuid.UUID       `json:"original_order_id"`
	Symbol          string          `json:"symbol"`
	NewPrice        decimal.Decimal `json:"new_price"`
	NewQuantity     decimal.Decimal `json:"new_quantity"`
	ResponseChan    chan *CancelReplaceResult `json:"-"` // Channel to synchronously report balance requirement back to gateway
}

// CancelReplaceResult encapsulates the result of the replacement.
type CancelReplaceResult struct {
	Success          bool
	Reason           string
	OldOrder         *Order
	NewOrder         *Order
	MatchResult      *MatchResult
	RequiredBalance  decimal.Decimal // Delta positive (needs more locked funds) or negative (releases funds)
	OldCost          decimal.Decimal // Cost of the old order
	NewCost          decimal.Decimal // Cost of the new order
}

// MatchResult encapsulates the output of matching an incoming order against the book.
type MatchResult struct {
	TakerOrder *Order   `json:"taker_order"` // The incoming order after matching
	Trades     []*Trade `json:"trades"`      // Zero or more trades produced
}
