// indicatorWorker.js
// Web Worker for high-performance Technical Analysis calculations without blocking the main thread.
// Uses Float64Arrays and Transferable Objects for zero-copy memory operations.

function calculateSMA(times, closes, period) {
  const len = closes.length;
  const smaValues = new Float64Array(len);
  
  let sum = 0;
  for (let i = 0; i < len; i++) {
    sum += closes[i];
    if (i >= period) {
      sum -= closes[i - period];
    }
    if (i >= period - 1) {
      smaValues[i] = sum / period;
    } else {
      smaValues[i] = NaN; // Not enough data
    }
  }
  return smaValues;
}

function calculateEMA(times, closes, period) {
  const len = closes.length;
  const emaValues = new Float64Array(len);
  
  if (len === 0) return emaValues;

  const multiplier = 2 / (period + 1);
  let ema = closes[0];
  emaValues[0] = ema;

  for (let i = 1; i < len; i++) {
    ema = (closes[i] - ema) * multiplier + ema;
    emaValues[i] = ema;
  }
  return emaValues;
}

function calculateRSI(times, closes, period) {
  const len = closes.length;
  const rsiValues = new Float64Array(len);
  
  if (len < period + 1) {
    rsiValues.fill(NaN);
    return rsiValues;
  }

  let avgGain = 0;
  let avgLoss = 0;

  // First avg gain/loss
  for (let i = 1; i <= period; i++) {
    const diff = closes[i] - closes[i - 1];
    if (diff > 0) avgGain += diff;
    else avgLoss += Math.abs(diff);
  }
  
  avgGain /= period;
  avgLoss /= period;
  
  for (let i = 0; i < period; i++) {
    rsiValues[i] = NaN;
  }
  
  if (avgLoss === 0) {
    rsiValues[period] = 100;
  } else {
    rsiValues[period] = 100 - (100 / (1 + (avgGain / avgLoss)));
  }

  for (let i = period + 1; i < len; i++) {
    const diff = closes[i] - closes[i - 1];
    const gain = diff > 0 ? diff : 0;
    const loss = diff < 0 ? Math.abs(diff) : 0;

    avgGain = (avgGain * (period - 1) + gain) / period;
    avgLoss = (avgLoss * (period - 1) + loss) / period;

    if (avgLoss === 0) {
      rsiValues[i] = 100;
    } else {
      rsiValues[i] = 100 - (100 / (1 + (avgGain / avgLoss)));
    }
  }
  
  return rsiValues;
}

function calculateMACD(times, closes, fastPeriod, slowPeriod, signalPeriod) {
  const len = closes.length;
  const fastEMA = calculateEMA(times, closes, fastPeriod);
  const slowEMA = calculateEMA(times, closes, slowPeriod);
  
  const macdLine = new Float64Array(len);
  for (let i = 0; i < len; i++) {
    if (isNaN(fastEMA[i]) || isNaN(slowEMA[i])) {
      macdLine[i] = NaN;
    } else {
      macdLine[i] = fastEMA[i] - slowEMA[i];
    }
  }

  // Calculate Signal Line (EMA of MACD Line)
  const signalLine = new Float64Array(len);
  const multiplier = 2 / (signalPeriod + 1);
  let ema = 0;
  let started = false;

  for (let i = 0; i < len; i++) {
    if (isNaN(macdLine[i])) {
      signalLine[i] = NaN;
      continue;
    }
    if (!started) {
      ema = macdLine[i];
      signalLine[i] = ema;
      started = true;
      continue;
    }
    ema = (macdLine[i] - ema) * multiplier + ema;
    signalLine[i] = ema;
  }

  const histogram = new Float64Array(len);
  for (let i = 0; i < len; i++) {
    if (isNaN(macdLine[i]) || isNaN(signalLine[i])) {
      histogram[i] = NaN;
    } else {
      histogram[i] = macdLine[i] - signalLine[i];
    }
  }

  return { macdLine, signalLine, histogram };
}

function calculateBollingerBands(times, closes, period, stdDevMultiplier) {
  const len = closes.length;
  const upper = new Float64Array(len);
  const lower = new Float64Array(len);
  const sma = calculateSMA(times, closes, period);
  
  for (let i = 0; i < len; i++) {
    if (i < period - 1) {
      upper[i] = NaN;
      lower[i] = NaN;
      continue;
    }
    let sum = 0;
    const mean = sma[i];
    for (let j = 0; j < period; j++) {
      const diff = closes[i - j] - mean;
      sum += diff * diff;
    }
    const stdDev = Math.sqrt(sum / period);
    upper[i] = mean + (stdDevMultiplier * stdDev);
    lower[i] = mean - (stdDevMultiplier * stdDev);
  }
  
  return { upper, lower };
}

// Listen for messages from the main thread
self.onmessage = function(e) {
  const { type, payload } = e.data;
  
  if (type === 'CALCULATE') {
    const { times, closes } = payload;
    
    // Perform calculations using the provided Float64Arrays
    const sma20 = calculateSMA(times, closes, 20);
    const ema9 = calculateEMA(times, closes, 9);
    const ema21 = calculateEMA(times, closes, 21);
    const ema50 = calculateEMA(times, closes, 50);
    const ema200 = calculateEMA(times, closes, 200);
    const rsi14 = calculateRSI(times, closes, 14);
    const macd = calculateMACD(times, closes, 12, 26, 9);
    const bb20_2 = calculateBollingerBands(times, closes, 20, 2);

    // Prepare transferables list (zero-copy memory transfer back to main thread)
    const transferables = [
      times.buffer,
      sma20.buffer,
      ema9.buffer,
      ema21.buffer,
      ema50.buffer,
      ema200.buffer,
      rsi14.buffer,
      macd.macdLine.buffer,
      macd.signalLine.buffer,
      macd.histogram.buffer,
      bb20_2.upper.buffer,
      bb20_2.lower.buffer
    ];

    self.postMessage({
      type: 'RESULT',
      payload: {
        times: times, // Return original times array buffer if needed, but it might have been transferred. 
                      // Wait, we need to be careful: if we transfer 'times', it's detached from worker, which is fine since we are done.
        sma20,
        ema9,
        ema21,
        ema50,
        ema200,
        rsi14,
        macd,
        bb20_2
      }
    }, transferables);
  }
};
