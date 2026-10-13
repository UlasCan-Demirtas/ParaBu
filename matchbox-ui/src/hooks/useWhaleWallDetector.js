import { useMemo } from 'react';

export function useWhaleWallDetector(asks, bids, currentPrice) {
  return useMemo(() => {
    const whaleWalls = {};
    
    // Calculate total volume for the active order book
    let totalVol = 0;
    asks.forEach(a => { if (a.price !== '-') totalVol += parseFloat(a.quantity); });
    bids.forEach(b => { totalVol += parseFloat(b.quantity); });
    
    if (totalVol === 0 || !currentPrice) return whaleWalls;

    const threshold = totalVol * 0.15; // 15% threshold for a single price level

    const processLevel = (level, type) => {
      if (level.price === '-') return;
      const qty = parseFloat(level.quantity);
      if (qty >= threshold) {
        const priceNum = parseFloat(level.price);
        const distance = Math.abs(currentPrice - priceNum) / currentPrice;
        const proximityAlert = distance <= 0.0005; // 0.05% proximity

        whaleWalls[level.price] = {
          type,
          qty,
          proximityAlert
        };
      }
    };

    asks.forEach(a => processLevel(a, 'ASK'));
    bids.forEach(b => processLevel(b, 'BID'));

    return whaleWalls;
  }, [asks, bids, currentPrice]);
}
