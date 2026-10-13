package marketdata

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

type Kline struct {
	OpenTime  int64   `json:"time"`  // mapped to lightweight-charts timestamp format (seconds)
	Open      float64 `json:"open"`
	High      float64 `json:"high"`
	Low       float64 `json:"low"`
	Close     float64 `json:"close"`
	Volume    float64 `json:"volume"`
	CloseTime int64   `json:"closeTime"`
}

type KlineService struct {
	client *http.Client
}

func NewKlineService() *KlineService {
	return &KlineService{
		client: &http.Client{
			// Basic timeout protection
			Timeout: 10 * time.Second,
		},
	}
}

func (s *KlineService) FetchKlines(ctx context.Context, symbol, interval string) ([]Kline, error) {
	// e.g. BTC/USDT -> BTCUSDT
	normalizedSymbol := strings.ReplaceAll(strings.ToUpper(symbol), "/", "")

	// Rate Limiting Protection via Context Deadline if caller desires, otherwise standard timeout
	url := fmt.Sprintf("https://api.binance.com/api/v3/klines?symbol=%s&interval=%s&limit=500", normalizedSymbol, interval)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("binance API returned status: %d", resp.StatusCode)
	}

	var rawData [][]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&rawData); err != nil {
		return nil, err
	}

	var klines []Kline
	for _, k := range rawData {
		if len(k) < 7 {
			continue
		}

		openTimeMs, _ := k[0].(float64)
		openStr, _ := k[1].(string)
		highStr, _ := k[2].(string)
		lowStr, _ := k[3].(string)
		closeStr, _ := k[4].(string)
		volStr, _ := k[5].(string)
		closeTimeMs, _ := k[6].(float64)

		open, _ := strconv.ParseFloat(openStr, 64)
		high, _ := strconv.ParseFloat(highStr, 64)
		low, _ := strconv.ParseFloat(lowStr, 64)
		closePrice, _ := strconv.ParseFloat(closeStr, 64)
		volume, _ := strconv.ParseFloat(volStr, 64)

		klines = append(klines, Kline{
			OpenTime:  int64(openTimeMs) / 1000,
			Open:      open,
			High:      high,
			Low:       low,
			Close:     closePrice,
			Volume:    volume,
			CloseTime: int64(closeTimeMs),
		})
	}

	return klines, nil
}
