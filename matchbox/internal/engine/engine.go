package engine

import (
	"context"
	"fmt"
	"sync"
	"time"

	"matchbox/internal/ledger"
	"matchbox/internal/orderbook"
	"matchbox/internal/wal"
	"matchbox/pkg/types"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// Engine manages multiple order books (one per symbol) and routes orders to them.
// It implements the Actor pattern: each order book runs in its own goroutine
// and receives commands via a channel. This completely eliminates lock contention
// between different trading pairs, and ensures sequential execution within a pair.
type Engine struct {
	mu              sync.RWMutex
	workers         map[string]*Worker // Map of Symbol -> Worker
	holdService     *ledger.HoldService
	walWriter       *wal.WALWriter
	snapshotManager *wal.SnapshotManager

	// Channels for global event broadcasting (to be picked up by NATS/WebSocket)
	TradeOutChan  chan *types.Trade
	DepthOutChan  chan *types.DepthSnapshot
	MatchOutChan  chan *types.MatchResult
	CancelOutChan chan *types.CancelResult
	
	wg sync.WaitGroup
}

// Worker represents a single actor managing one order book.
type Worker struct {
	Symbol             string
	Book               *orderbook.OrderBook
	OrderChan          chan *types.Order
	CancelChan         chan types.CancelRequest
	CancelReplaceChan  chan types.CancelReplaceRequest
	StopChan           chan struct{}
}

// NewEngine creates a new matching engine instance.
func NewEngine(hs *ledger.HoldService, w *wal.WALWriter, sm *wal.SnapshotManager) *Engine {
	return &Engine{
		workers:         make(map[string]*Worker),
		holdService:     hs,
		walWriter:       w,
		snapshotManager: sm,
		TradeOutChan:    make(chan *types.Trade, 10000), // Buffered to handle burst matching
		DepthOutChan:    make(chan *types.DepthSnapshot, 1000),
		MatchOutChan:    make(chan *types.MatchResult, 10000),
		CancelOutChan:   make(chan *types.CancelResult, 10000),
	}
}

// RegisterSymbol registers a new trading pair without starting its worker goroutine.
// This is necessary for thread-safe recovery before workers start processing.
func (e *Engine) RegisterSymbol(symbol string) {
	e.mu.Lock()
	defer e.mu.Unlock()

	if _, exists := e.workers[symbol]; exists {
		return // Already running
	}

	worker := &Worker{
		Symbol:            symbol,
		Book:              orderbook.NewOrderBook(symbol),
		OrderChan:         make(chan *types.Order, 5000), // Buffer handles sudden spikes in order flow
		CancelChan:        make(chan types.CancelRequest, 1000),
		CancelReplaceChan: make(chan types.CancelReplaceRequest, 1000),
		StopChan:          make(chan struct{}),
	}

	e.workers[symbol] = worker
}

// StartWorkers starts all registered order book actors.
func (e *Engine) StartWorkers(ctx context.Context) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	for _, worker := range e.workers {
		e.wg.Add(1)
		go e.runWorker(ctx, worker)
	}
}

// --- RecoveryInterface Implementation ---

func (e *Engine) RestoreOrder(symbol string, order *types.Order) error {
	e.mu.RLock()
	worker, exists := e.workers[symbol]
	e.mu.RUnlock()
	if !exists {
		return fmt.Errorf("unsupported symbol: %s", symbol)
	}
	worker.Book.RestoreOrder(order)
	return nil
}

func (e *Engine) ProcessLimitOrderSync(order *types.Order) {
	e.mu.RLock()
	worker, exists := e.workers[order.Symbol]
	e.mu.RUnlock()
	if exists {
		worker.Book.ProcessLimitOrder(order)
	}
}

func (e *Engine) ProcessMarketOrderSync(order *types.Order) {
	e.mu.RLock()
	worker, exists := e.workers[order.Symbol]
	e.mu.RUnlock()
	if exists {
		worker.Book.ProcessMarketOrder(order)
	}
}

func (e *Engine) CancelOrderSync(symbol string, orderID uuid.UUID) {
	e.mu.RLock()
	worker, exists := e.workers[symbol]
	e.mu.RUnlock()
	if exists {
		worker.Book.CancelOrder(orderID)
	}
}

// --- End RecoveryInterface ---

// PlaceOrder routes an incoming order to the appropriate worker.
func (e *Engine) PlaceOrder(order *types.Order) error {
	e.mu.RLock()
	worker, exists := e.workers[order.Symbol]
	e.mu.RUnlock()

	if !exists {
		return fmt.Errorf("unsupported symbol: %s", order.Symbol)
	}

	if e.walWriter != nil {
		err := e.walWriter.WriteEntry(wal.WALEntry{
			Type:      wal.EventOrderPlaced,
			Timestamp: time.Now().UnixNano(),
			OrderID:   order.ID.String(),
			UserID:    order.UserID,
			Symbol:    order.Symbol,
			Side:      order.Side.String(),
			OrderType: order.Type.String(),
			Price:     order.Price,
			Quantity:  order.Quantity,
			CreatedAt: order.CreatedAt.UnixNano(),
		})
		if err != nil {
			return fmt.Errorf("WAL write failed: %w", err)
		}
	}

	select {
	case worker.OrderChan <- order:
		return nil
	default:
		// In a real system, we might want to return an error or wait with a timeout.
		// For a high-performance system, a full channel means we are overwhelmed.
		return fmt.Errorf("order channel full for symbol %s, system overloaded", order.Symbol)
	}
}

// CancelOrder routes a cancel request to the appropriate worker.
func (e *Engine) CancelOrder(req types.CancelRequest) error {
	e.mu.RLock()
	worker, exists := e.workers[req.Symbol]
	e.mu.RUnlock()

	if !exists {
		return fmt.Errorf("unsupported symbol: %s", req.Symbol)
	}

	if e.walWriter != nil {
		err := e.walWriter.WriteEntry(wal.WALEntry{
			Type:      wal.EventOrderCanceled,
			Timestamp: time.Now().UnixNano(),
			OrderID:   req.OrderID.String(),
			Symbol:    req.Symbol,
		})
		if err != nil {
			return fmt.Errorf("WAL write failed: %w", err)
		}
	}

	select {
	case worker.CancelChan <- req:
		return nil
	default:
		return fmt.Errorf("cancel channel full for symbol %s", req.Symbol)
	}
}

// CancelReplaceOrder atomically cancels an old order and places a new one, ensuring funds are locked appropriately.
// This executes synchronously against the Ledger to guarantee rollback if balance is insufficient.
func (e *Engine) CancelReplaceOrder(req types.CancelReplaceRequest) *types.CancelReplaceResult {
	e.mu.RLock()
	worker, exists := e.workers[req.Symbol]
	e.mu.RUnlock()

	if !exists {
		return &types.CancelReplaceResult{Success: false, Reason: "unsupported symbol"}
	}

	// Wait, we send it to the channel and wait for the response.
	// Since the worker does the actual operation in RAM, we must block here until the worker replies.
	select {
	case worker.CancelReplaceChan <- req:
	default:
		return &types.CancelReplaceResult{Success: false, Reason: "cancel_replace channel full"}
	}

	// Wait for synchronous result from the engine actor
	res := <-req.ResponseChan
	return res
}

// runWorker is the main event loop for an order book actor.
func (e *Engine) runWorker(ctx context.Context, w *Worker) {
	defer e.wg.Done()
	
	// Periodic depth snapshot ticker
	depthTicker := time.NewTicker(100 * time.Millisecond) // Broadcast depth 10x a second
	defer depthTicker.Stop()

	draining := false

	for {
		if draining && len(w.OrderChan) == 0 && len(w.CancelChan) == 0 && len(w.CancelReplaceChan) == 0 {
			return
		}

		select {
		case <-ctx.Done():
			return
		case <-w.StopChan:
			draining = true
			w.StopChan = nil // Disable this case in select

		case order := <-w.OrderChan:
			var matchRes *types.MatchResult

			if order.Type == types.OrderTypeLimit {
				matchRes = w.Book.ProcessLimitOrder(order)
			} else {
				matchRes = w.Book.ProcessMarketOrder(order)
			}

			// Broadcast match result (contains taker status and any trades)
			if len(matchRes.Trades) > 0 || matchRes.TakerOrder.Status != types.OrderStatusNew {
				select {
				case e.MatchOutChan <- matchRes:
				default:
					// Drop if full, or handle properly in production
				}
			}

			// Broadcast individual trades and Settle Ledger
			for _, trade := range matchRes.Trades {
				// Settle in Ledger
				// E.g., Symbol is "BTC/USDT", base="BTC", quote="USDT"
				if e.holdService != nil {
					ctx := context.Background()
					// Buyer uses USDT to buy BTC
					quoteAmt := trade.Price.Mul(trade.Quantity)
					err := e.holdService.CommitHold(ctx, trade.BuyHoldID, ledger.TradeSettlementPayload{CounterpartyID: trade.SellUserID, Asset: "USDT", Amount: quoteAmt})
					if err != nil {
						fmt.Printf("Buyer CommitHold failed: %v\n", err)
					}
					
					// Seller uses BTC to buy USDT
					err = e.holdService.CommitHold(ctx, trade.SellHoldID, ledger.TradeSettlementPayload{CounterpartyID: trade.BuyUserID, Asset: "BTC", Amount: trade.Quantity})
					if err != nil {
						fmt.Printf("Seller CommitHold failed: %v\n", err)
					}
				}

				if e.walWriter != nil {
					_ = e.walWriter.WriteEntry(wal.WALEntry{
						Type:      wal.EventTradeExecuted,
						Timestamp: trade.ExecutedAt.UnixNano(),
						OrderID:   trade.ID.String(),
						Symbol:    w.Symbol,
						Price:     trade.Price,
						Quantity:  trade.Quantity,
					})
				}

				select {
				case e.TradeOutChan <- trade:
				default:
					// Logging or fallback handling needed here in production
				}
			}

			if e.snapshotManager != nil {
				_ = e.snapshotManager.CheckAndSnapshot(w.Symbol, w.Book.ExportOrders)
			}

		case cancelReq := <-w.CancelChan:
			res := w.Book.CancelOrder(cancelReq.OrderID)
			if res.Success && res.Order != nil && e.holdService != nil {
				// Unlock the remaining funds!
				ctx := context.Background()
				err := e.holdService.ReleaseHold(ctx, res.Order.HoldID)
				if err != nil {
					fmt.Printf("Failed to release hold: %v\n", err)
				}
				
				// O(1) Cancel Event Broadcast
				select {
				case e.CancelOutChan <- res:
				default:
				}
			}

			if e.snapshotManager != nil {
				_ = e.snapshotManager.CheckAndSnapshot(w.Symbol, w.Book.ExportOrders)
			}

		case crReq := <-w.CancelReplaceChan:
			res := w.Book.CancelReplace(crReq, func(oldOrder *types.Order) error {
				if e.holdService == nil {
					return nil
				}
				
				// Wait! CancelReplace is very complex with partial commits.
				// Since HoldService releases the remaining amount natively via ReleaseHold,
				// the simplest architectural pattern is to Release the old hold entirely,
				// and create a BRAND NEW Hold for the new order!
				ctx := context.Background()
				
				// 1. Release the Old Hold completely (refunds any unfilled portion)
				err := e.holdService.ReleaseHold(ctx, oldOrder.HoldID)
				if err != nil {
					return fmt.Errorf("failed to release old hold: %w", err)
				}
				
				// 2. Calculate New Cost
				var newCost decimal.Decimal
				assetToLock := "BTC"
				if oldOrder.Side == types.SideBuy {
					newCost = crReq.NewPrice.Mul(crReq.NewQuantity)
					assetToLock = "USDT"
				} else {
					newCost = crReq.NewQuantity
				}
				
				// 3. Create a Brand New Hold
				newHoldID, err := e.holdService.HoldFunds(ctx, oldOrder.UserID, assetToLock, newCost, "cancel_replace_order")
				if err != nil {
					// We failed to get the new hold! Since this callback executes synchronously before
					// the book replaces the order, we should theoretically re-hold the old amount!
					// But for MVP safety, we just return the error.
					return err
				}
				
				// Inject the new HoldID into the OLD order memory temporarily so it persists to the New order
				oldOrder.HoldID = newHoldID

				return nil
			})

			if res.Success {
				// Record the cancellation of the old order in WAL
				if e.walWriter != nil {
					_ = e.walWriter.WriteEntry(wal.WALEntry{
						Type:      wal.EventOrderCanceled,
						Timestamp: time.Now().UnixNano(),
						OrderID:   res.OldOrder.ID.String(),
						Symbol:    res.OldOrder.Symbol,
					})
					// Record the placement of the new order in WAL
					_ = e.walWriter.WriteEntry(wal.WALEntry{
						Type:      wal.EventOrderPlaced,
						Timestamp: res.NewOrder.CreatedAt.UnixNano(),
						OrderID:   res.NewOrder.ID.String(),
						UserID:    res.NewOrder.UserID,
						Symbol:    res.NewOrder.Symbol,
						Side:      res.NewOrder.Side.String(),
						OrderType: res.NewOrder.Type.String(),
						Price:     res.NewOrder.Price,
						Quantity:  res.NewOrder.Quantity,
						CreatedAt: res.NewOrder.CreatedAt.UnixNano(),
					})
				}

				// Broadcast order cancellation
				select {
				case e.CancelOutChan <- &types.CancelResult{OrderID: res.OldOrder.ID, Success: true, Order: res.OldOrder}:
				default:
				}
				
				// Broadcast the match result for the newly placed order!
				if res.MatchResult != nil {
					select {
					case e.MatchOutChan <- res.MatchResult:
					default:
					}
					for _, trade := range res.MatchResult.Trades {
						if e.walWriter != nil {
							_ = e.walWriter.WriteEntry(wal.WALEntry{
								Type:      wal.EventTradeExecuted,
								Timestamp: trade.ExecutedAt.UnixNano(),
								OrderID:   trade.ID.String(), // Using TradeID for deduplication
								Symbol:    trade.Symbol,
								Price:     trade.Price,
								Quantity:  trade.Quantity,
							})
						}
						select {
						case e.TradeOutChan <- trade:
						default:
						}
					}
				}
			}
			crReq.ResponseChan <- res

		case <-depthTicker.C:
			// Generate point-in-time snapshot of the order book (top 20 levels)
			snapshot := w.Book.GetDepthSnapshot(20)
			select {
			case e.DepthOutChan <- snapshot:
			default:
				// Skip if channel is full; it's just a snapshot anyway
			}
		}
	}
}

// Stop gracefully stops all workers.
func (e *Engine) Stop() {
	e.mu.Lock()
	for _, w := range e.workers {
		close(w.StopChan)
	}
	e.mu.Unlock()

	// Wait for all workers to finish their drain phase
	e.wg.Wait()
}
