package marketmaker

import (
	"context"
	"encoding/json"
	"math"
	"math/rand"
	"os"
	"strconv"
	"time"

	"matchbox/internal/engine"
	"matchbox/internal/ledger"
	"matchbox/internal/ws"
	"matchbox/pkg/types"

	"github.com/redis/go-redis/v9"
	"github.com/shopspring/decimal"
	"go.uber.org/zap"
)

type SandboxConfig struct {
	Enabled       bool
	Volatility    float64
	Drift         float64
	CleanupOnExit bool
}

type Kline struct {
	OpenTime int64           `json:"open_time"` // Start of the interval
	Open     decimal.Decimal `json:"open"`
	High     decimal.Decimal `json:"high"`
	Low      decimal.Decimal `json:"low"`
	Close    decimal.Decimal `json:"close"`
	Volume   decimal.Decimal `json:"volume"`
}

func loadConfig() SandboxConfig {
	enabled := os.Getenv("SANDBOX_MODE") == "true"
	vol, _ := strconv.ParseFloat(os.Getenv("SANDBOX_VOLATILITY"), 64)
	if vol == 0 {
		vol = 0.05 // default 5%
	}
	drift, _ := strconv.ParseFloat(os.Getenv("SANDBOX_DRIFT"), 64)
	if drift == 0 {
		drift = 0.0001 // slight upward drift default
	}
	cleanup := os.Getenv("SANDBOX_CLEANUP_ON_EXIT") == "true"

	return SandboxConfig{
		Enabled:       enabled,
		Volatility:    vol,
		Drift:         drift,
		CleanupOnExit: cleanup,
	}
}

// StartSimulator runs the Geometric Brownian Motion sandbox
func StartSimulator(ctx context.Context, eng *engine.Engine, hs *ledger.HoldService, l *ledger.Ledger, rdb *redis.Client, hub *ws.Hub, logger *zap.SugaredLogger) {
	cfg := loadConfig()
	if !cfg.Enabled {
		logger.Info("Sandbox Mode is DISABLED.")
		return
	}

	logger.Infof("Starting GBM Sandbox Simulator... Volatility: %.4f, Drift: %.4f", cfg.Volatility, cfg.Drift)

	// Seed bot-market-maker with massive balances so LockFunds never fails
	botAssets := []string{"USDT", "BTC", "ETH", "XRP"}
	for _, asset := range botAssets {
		l.Deposit("bot-market-maker", asset, decimal.NewFromInt(1_000_000_000))
	}
	logger.Info("Seeded bot-market-maker with 1B of each asset")

	// Seed test user with starting balances
	l.Deposit("user-1", "USDT", decimal.NewFromInt(100_000))
	l.Deposit("user-1", "BTC", decimal.NewFromInt(10))
	l.Deposit("user-1", "ETH", decimal.NewFromInt(100))
	l.Deposit("user-1", "XRP", decimal.NewFromInt(10_000))
	logger.Info("Seeded user-1 with test balances")

	symbols := []string{"BTC/USDT", "ETH/USDT"}
	basePrices := map[string]float64{
		"BTC/USDT": 60000.0,
		"ETH/USDT": 3000.0,
	}

	// For Kline aggregation
	current1sKlines := make(map[string]*Kline)
	current1mKlines := make(map[string]*Kline)

	ticker := time.NewTicker(100 * time.Millisecond)

	// dt is fractional year. Assuming 252 trading days * 24 * 60 * 60 = 21,772,800 seconds.
	// 100ms = 0.1 seconds = 0.1 / 21772800 = 4.59e-9
	dt := 0.1 / 21772800.0

	// We don't want strict financial dt, just visually appealing random walks
	// We'll scale dt to make 100ms visible
	dt = 1.0 / 365.0 / 24.0 / 60.0 / 60.0 / 10.0 // 10 ticks per second

	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				if cfg.CleanupOnExit {
					logger.Info("Sandbox Graceful Cleanup: Wiping mock klines from Redis...")
					for _, sym := range symbols {
						rdb.Del(context.Background(), "sandbox:klines:1s:"+sym)
						rdb.Del(context.Background(), "sandbox:klines:1m:"+sym)
					}
				}
				return
			case t := <-ticker.C:
				ts := t.Unix()
				ts1m := ts - (ts % 60)
				logger.Infof("Sandbox ticker fired at %v", ts)

				for sym, p := range basePrices {
					baseAsset := sym[:len(sym)-5]

					// 1. Calculate new price via GBM
					// dS = S * (mu * dt + sigma * dW)
					dW := rand.NormFloat64() * math.Sqrt(dt)
					driftTerm := cfg.Drift * dt
					volTerm := cfg.Volatility * dW
					
					newP := p * math.Exp(driftTerm - 0.5*cfg.Volatility*cfg.Volatility*dt + volTerm)
					basePrices[sym] = newP

					// 2. Inject Liquidity (Bid/Ask) around newP
					askPrice := decimal.NewFromFloat(newP * 1.0001)
					bidPrice := decimal.NewFromFloat(newP * 0.9999)
					qty := decimal.NewFromFloat(0.1 + rand.Float64()*0.4)

					// Lock funds for bot
					holdAsk, errLockAsk := hs.HoldFunds(context.Background(), "bot-market-maker", baseAsset, qty, "sandbox_ask")
					if errLockAsk != nil {
						logger.Errorf("Failed to lock %s for ask: %v", baseAsset, errLockAsk)
					}
					askOrder := types.NewOrder("bot-market-maker", sym, types.SideSell, types.OrderTypeLimit, askPrice, qty)
					askOrder.HoldID = holdAsk
					errAsk := eng.PlaceOrder(askOrder)
					if errAsk != nil {
						logger.Errorf("Failed to place ask order: %v", errAsk)
					}

					holdBid, errLockBid := hs.HoldFunds(context.Background(), "bot-market-maker", "USDT", bidPrice.Mul(qty), "sandbox_bid")
					if errLockBid != nil {
						logger.Errorf("Failed to lock USDT for bid: %v", errLockBid)
					}
					bidOrder := types.NewOrder("bot-market-maker", sym, types.SideBuy, types.OrderTypeLimit, bidPrice, qty)
					bidOrder.HoldID = holdBid
					errBid := eng.PlaceOrder(bidOrder)
					if errBid != nil {
						logger.Errorf("Failed to place bid order: %v", errBid)
					}
					
					logger.Infof("Sandbox placed bid/ask for %s at %.2f", sym, newP)

					// 3. Update K-Lines
					currentPriceDec := decimal.NewFromFloat(newP)
					
					// 1-second Kline
					if k1s, ok := current1sKlines[sym]; !ok || k1s.OpenTime != ts {
						if ok {
							// Push completed 1s kline to Redis and Broadcast
							klineJSON, _ := json.Marshal(k1s)
							rdb.RPush(ctx, "sandbox:klines:1s:"+sym, klineJSON)
							hub.Broadcast("kline:1s:"+sym, k1s)
							
							// Ring buffer: keep last 3600 seconds (1h)
							rdb.LTrim(ctx, "sandbox:klines:1s:"+sym, -3600, -1)
						}
						current1sKlines[sym] = &Kline{
							OpenTime: ts,
							Open:     currentPriceDec,
							High:     currentPriceDec,
							Low:      currentPriceDec,
							Close:    currentPriceDec,
							Volume:   qty,
						}
					} else {
						k1s.Close = currentPriceDec
						if currentPriceDec.GreaterThan(k1s.High) { k1s.High = currentPriceDec }
						if currentPriceDec.LessThan(k1s.Low) { k1s.Low = currentPriceDec }
						k1s.Volume = k1s.Volume.Add(qty)
						// Live Broadcast
						hub.Broadcast("kline:1s:"+sym, k1s)
					}

					// 1-minute Kline
					if k1m, ok := current1mKlines[sym]; !ok || k1m.OpenTime != ts1m {
						if ok {
							// Push completed 1m kline to Redis and Broadcast
							klineJSON, _ := json.Marshal(k1m)
							rdb.RPush(ctx, "sandbox:klines:1m:"+sym, klineJSON)
							hub.Broadcast("kline:1m:"+sym, k1m)

							// Ring buffer: keep last 1440 minutes (24h)
							rdb.LTrim(ctx, "sandbox:klines:1m:"+sym, -1440, -1)
						}
						current1mKlines[sym] = &Kline{
							OpenTime: ts1m,
							Open:     currentPriceDec,
							High:     currentPriceDec,
							Low:      currentPriceDec,
							Close:    currentPriceDec,
							Volume:   qty,
						}
					} else {
						k1m.Close = currentPriceDec
						if currentPriceDec.GreaterThan(k1m.High) { k1m.High = currentPriceDec }
						if currentPriceDec.LessThan(k1m.Low) { k1m.Low = currentPriceDec }
						k1m.Volume = k1m.Volume.Add(qty)
						// Live Broadcast
						hub.Broadcast("kline:1m:"+sym, k1m)
					}
				}
			}
		}
	}()
}
