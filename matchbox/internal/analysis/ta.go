package analysis

import (
	"matchbox/internal/marketdata"
)

// TAData point represents a single time slice of technical analysis metrics.
type TAData struct {
	Time  int64   `json:"time"`
	Value float64 `json:"value"`
}

type MACDData struct {
	Time      int64   `json:"time"`
	MACD      float64 `json:"macd"`
	Signal    float64 `json:"signal"`
	Histogram float64 `json:"histogram"`
}

type TAService struct{}

func NewTAService() *TAService {
	return &TAService{}
}

// CalculateSMA calculates the Simple Moving Average over period N.
func (s *TAService) CalculateSMA(klines []marketdata.Kline, period int) []TAData {
	if len(klines) < period || period <= 0 {
		return []TAData{}
	}

	result := make([]TAData, 0, len(klines)-period+1)
	
	// Initial sum
	sum := 0.0
	for i := 0; i < period; i++ {
		sum += klines[i].Close
	}
	result = append(result, TAData{
		Time:  klines[period-1].OpenTime,
		Value: sum / float64(period),
	})

	// Sliding window
	for i := period; i < len(klines); i++ {
		sum = sum - klines[i-period].Close + klines[i].Close
		result = append(result, TAData{
			Time:  klines[i].OpenTime,
			Value: sum / float64(period),
		})
	}

	return result
}

// CalculateEMA calculates the Exponential Moving Average over period N.
func (s *TAService) CalculateEMA(klines []marketdata.Kline, period int) []TAData {
	if len(klines) < period || period <= 0 {
		return []TAData{}
	}

	result := make([]TAData, 0, len(klines)-period+1)
	
	// Initial EMA is the SMA of the first 'period' elements
	sum := 0.0
	for i := 0; i < period; i++ {
		sum += klines[i].Close
	}
	initialSMA := sum / float64(period)
	result = append(result, TAData{
		Time:  klines[period-1].OpenTime,
		Value: initialSMA,
	})

	multiplier := 2.0 / float64(period+1)
	prevEMA := initialSMA

	for i := period; i < len(klines); i++ {
		currentEMA := (klines[i].Close-prevEMA)*multiplier + prevEMA
		result = append(result, TAData{
			Time:  klines[i].OpenTime,
			Value: currentEMA,
		})
		prevEMA = currentEMA
	}

	return result
}

// _calculateEMA is an internal helper that takes float64 slice and returns float64 slice.
func _calculateEMA(values []float64, period int) []float64 {
	if len(values) < period || period <= 0 {
		return []float64{}
	}

	result := make([]float64, 0, len(values)-period+1)
	
	sum := 0.0
	for i := 0; i < period; i++ {
		sum += values[i]
	}
	prevEMA := sum / float64(period)
	result = append(result, prevEMA)

	multiplier := 2.0 / float64(period+1)
	for i := period; i < len(values); i++ {
		currentEMA := (values[i]-prevEMA)*multiplier + prevEMA
		result = append(result, currentEMA)
		prevEMA = currentEMA
	}
	return result
}

// CalculateMACD calculates MACD (default 12, 26, 9)
func (s *TAService) CalculateMACD(klines []marketdata.Kline, fastPeriod, slowPeriod, signalPeriod int) []MACDData {
	if len(klines) < slowPeriod {
		return []MACDData{}
	}

	fastEMA := s.CalculateEMA(klines, fastPeriod)
	slowEMA := s.CalculateEMA(klines, slowPeriod)

	// fastEMA will have more elements than slowEMA since fastPeriod < slowPeriod.
	// Align them. The first element of slowEMA corresponds to index (slowPeriod - 1).
	// The element at this index in fastEMA is at fastEMA index: (slowPeriod - 1) - (fastPeriod - 1) = slowPeriod - fastPeriod
	
	diff := slowPeriod - fastPeriod
	var macdLine []float64
	var times []int64

	for i := 0; i < len(slowEMA); i++ {
		macdVal := fastEMA[i+diff].Value - slowEMA[i].Value
		macdLine = append(macdLine, macdVal)
		times = append(times, slowEMA[i].Time)
	}

	if len(macdLine) < signalPeriod {
		return []MACDData{}
	}

	signalLine := _calculateEMA(macdLine, signalPeriod)
	sigDiff := len(macdLine) - len(signalLine)

	result := make([]MACDData, 0, len(signalLine))
	for i := 0; i < len(signalLine); i++ {
		macd := macdLine[i+sigDiff]
		sig := signalLine[i]
		hist := macd - sig
		result = append(result, MACDData{
			Time:      times[i+sigDiff],
			MACD:      macd,
			Signal:    sig,
			Histogram: hist,
		})
	}

	return result
}

// CalculateRSI calculates the Relative Strength Index over period N.
func (s *TAService) CalculateRSI(klines []marketdata.Kline, period int) []TAData {
	if len(klines) <= period || period <= 0 {
		return []TAData{}
	}

	result := make([]TAData, 0, len(klines)-period)
	
	var sumGain, sumLoss float64

	// First calculate average gain and loss over the initial period
	for i := 1; i <= period; i++ {
		change := klines[i].Close - klines[i-1].Close
		if change > 0 {
			sumGain += change
		} else {
			sumLoss -= change // make positive
		}
	}

	avgGain := sumGain / float64(period)
	avgLoss := sumLoss / float64(period)
	
	var rsi float64
	if avgLoss == 0 {
		rsi = 100.0
	} else {
		rs := avgGain / avgLoss
		rsi = 100.0 - (100.0 / (1.0 + rs))
	}
	
	result = append(result, TAData{
		Time:  klines[period].OpenTime,
		Value: rsi,
	})

	// Smoothed moving average for the rest
	for i := period + 1; i < len(klines); i++ {
		change := klines[i].Close - klines[i-1].Close
		gain := 0.0
		loss := 0.0
		if change > 0 {
			gain = change
		} else {
			loss = -change
		}

		avgGain = (avgGain*float64(period-1) + gain) / float64(period)
		avgLoss = (avgLoss*float64(period-1) + loss) / float64(period)

		if avgLoss == 0 {
			rsi = 100.0
		} else {
			rs := avgGain / avgLoss
			rsi = 100.0 - (100.0 / (1.0 + rs))
		}

		result = append(result, TAData{
			Time:  klines[i].OpenTime,
			Value: rsi,
		})
	}

	return result
}
