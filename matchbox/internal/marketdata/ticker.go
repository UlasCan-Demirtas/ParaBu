package marketdata

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/shopspring/decimal"
)

type Ticker24h struct {
	Symbol      string          `json:"symbol"`
	High        decimal.Decimal `json:"high"`
	Low         decimal.Decimal `json:"low"`
	Volume      decimal.Decimal `json:"volume"`
	ChangePct   decimal.Decimal `json:"change_pct"`
	VWAP        decimal.Decimal `json:"vwap"`
	LastPrice   decimal.Decimal `json:"last_price"`
	OpenPrice   decimal.Decimal `json:"open_price"`
	Count       int64           `json:"count"`
}

type TradeEntry struct {
	Price    decimal.Decimal `json:"p"`
	Quantity decimal.Decimal `json:"q"`
	Time     int64           `json:"t"`
}

type TickerService struct {
	rdb *redis.Client
}

func NewTickerService(rdb *redis.Client) *TickerService {
	return &TickerService{rdb: rdb}
}

// ProcessTrade adds a trade to the 24h ZSET, prunes old trades, calculates the 24h stats, and caches the result.
func (s *TickerService) ProcessTrade(ctx context.Context, symbol string, price, quantity decimal.Decimal) error {
	now := time.Now().UnixMilli()
	windowStart := now - 24*60*60*1000

	zsetKey := fmt.Sprintf("trades:24h:%s", symbol)
	tickerKey := fmt.Sprintf("ticker:24h:%s", symbol)

	tradeData, _ := json.Marshal(TradeEntry{Price: price, Quantity: quantity, Time: now})

	pipe := s.rdb.Pipeline()
	// Remove trades older than 24h
	pipe.ZRemRangeByScore(ctx, zsetKey, "0", fmt.Sprintf("%d", windowStart))
	// Add new trade
	pipe.ZAdd(ctx, zsetKey, redis.Z{
		Score:  float64(now),
		Member: string(tradeData),
	})
	// Fetch remaining valid trades
	pipe.ZRange(ctx, zsetKey, 0, -1)
	
	cmds, err := pipe.Exec(ctx)
	if err != nil {
		return err
	}

	// Calculate Stats
	tradesRaw := cmds[2].(*redis.StringSliceCmd).Val()
	if len(tradesRaw) == 0 {
		return nil
	}

	var high, low, volume, quoteVolume, openPrice, lastPrice decimal.Decimal
	
	for i, raw := range tradesRaw {
		var t TradeEntry
		json.Unmarshal([]byte(raw), &t)

		if i == 0 {
			openPrice = t.Price
			high = t.Price
			low = t.Price
		} else {
			if t.Price.GreaterThan(high) {
				high = t.Price
			}
			if t.Price.LessThan(low) {
				low = t.Price
			}
		}
		
		volume = volume.Add(t.Quantity)
		quoteVolume = quoteVolume.Add(t.Price.Mul(t.Quantity))
		lastPrice = t.Price
	}

	changePct := decimal.Zero
	if !openPrice.IsZero() {
		changePct = lastPrice.Sub(openPrice).Div(openPrice).Mul(decimal.NewFromInt(100))
	}

	vwap := decimal.Zero
	if !volume.IsZero() {
		vwap = quoteVolume.Div(volume)
	}

	ticker := Ticker24h{
		Symbol:    symbol,
		High:      high,
		Low:       low,
		Volume:    volume,
		ChangePct: changePct,
		VWAP:      vwap,
		LastPrice: lastPrice,
		OpenPrice: openPrice,
		Count:     int64(len(tradesRaw)),
	}

	tickerJSON, _ := json.Marshal(ticker)
	
	// Store aggregated ticker in O(1) accessible Redis String
	return s.rdb.Set(ctx, tickerKey, tickerJSON, 0).Err()
}

// GetTicker returns the cached 24h ticker in O(1) time
func (s *TickerService) GetTicker(ctx context.Context, symbol string) (*Ticker24h, error) {
	tickerKey := fmt.Sprintf("ticker:24h:%s", symbol)
	res, err := s.rdb.Get(ctx, tickerKey).Result()
	if err != nil {
		return nil, err
	}
	
	var ticker Ticker24h
	if err := json.Unmarshal([]byte(res), &ticker); err != nil {
		return nil, err
	}
	
	return &ticker, nil
}
