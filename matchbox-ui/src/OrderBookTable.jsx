import React, { useMemo } from 'react';

const OrderBookTable = ({ bids, asks, onSelectPrice }) => {
  // Compute cumulative volumes and maximum volume for scaling
  const processedAsks = useMemo(() => {
    let cumulative = 0;
    // Asks are usually ordered by price ascending. We want to display them with the highest price at the top.
    // So we iterate backwards to compute cumulative volume starting from the lowest price (which will be at the bottom).
    const reversed = [...asks].reverse();
    const result = reversed.map((ask) => {
      const qty = parseFloat(ask.quantity);
      cumulative += qty;
      return { ...ask, cumulative, qty };
    }).reverse(); // Reverse back to render top-down
    return result;
  }, [asks]);

  const processedBids = useMemo(() => {
    let cumulative = 0;
    // Bids are usually ordered by price descending. Highest price at the top.
    return bids.map((bid) => {
      const qty = parseFloat(bid.quantity);
      cumulative += qty;
      return { ...bid, cumulative, qty };
    });
  }, [bids]);

  const maxVolume = useMemo(() => {
    const maxAsk = processedAsks.length > 0 ? processedAsks[0].cumulative : 0; 
    const maxBid = processedBids.length > 0 ? processedBids[processedBids.length - 1].cumulative : 0;
    return Math.max(maxAsk, maxBid, 1); // fallback to 1 to avoid division by zero
  }, [processedAsks, processedBids]);

  return (
    <div className="flex flex-col h-full bg-[#050505] font-mono text-[11px] overflow-hidden">
      {/* Header */}
      <div className="flex justify-between items-center px-3 py-2 text-[#888888] font-semibold border-b border-[#1A1A1A]">
        <span className="w-1/3 text-left">Fiyat (USDT)</span>
        <span className="w-1/3 text-right">Miktar (BTC)</span>
        <span className="w-1/3 text-right">Toplam (BTC)</span>
      </div>

      <div className="flex-1 overflow-y-auto custom-scrollbar flex flex-col">
        {/* Asks (Sells) */}
        <div className="flex flex-col justify-end flex-1">
          {processedAsks.map((ask, i) => {
            const widthPct = Math.min((ask.cumulative / maxVolume) * 100, 100);
            return (
              <div 
                key={`ask-${i}`} 
                onClick={() => onSelectPrice && onSelectPrice(ask.price)}
                className="relative flex justify-between items-center px-3 py-[2px] cursor-pointer hover:bg-[#111111] transition-colors group"
              >
                {/* Background Depth Bar */}
                <div 
                  className="absolute right-0 top-0 bottom-0 pointer-events-none transition-all duration-200"
                  style={{ 
                    width: `${widthPct}%`, 
                    backgroundColor: 'rgba(255, 23, 68, 0.15)' 
                  }}
                />
                <span className="text-[#FF1744] font-bold w-1/3 text-left relative z-10">{parseFloat(ask.price).toFixed(2)}</span>
                <span className="text-[#E0E0E0] w-1/3 text-right relative z-10">{ask.qty.toFixed(4)}</span>
                <span className="text-[#888888] w-1/3 text-right relative z-10">{ask.cumulative.toFixed(4)}</span>
              </div>
            );
          })}
        </div>

        {/* Spread / Mid Market Price */}
        <div className="py-2 px-3 border-y border-[#1A1A1A] bg-[#0A0A0A] flex justify-center items-center gap-2">
          <span className="text-white text-sm font-bold tracking-wider">
            {processedAsks.length > 0 ? parseFloat(processedAsks[processedAsks.length - 1].price).toFixed(2) : '---'}
          </span>
          <span className="text-[#888888] text-[10px]">USD</span>
        </div>

        {/* Bids (Buys) */}
        <div className="flex flex-col flex-1">
          {processedBids.map((bid, i) => {
            const widthPct = Math.min((bid.cumulative / maxVolume) * 100, 100);
            return (
              <div 
                key={`bid-${i}`} 
                onClick={() => onSelectPrice && onSelectPrice(bid.price)}
                className="relative flex justify-between items-center px-3 py-[2px] cursor-pointer hover:bg-[#111111] transition-colors group"
              >
                {/* Background Depth Bar */}
                <div 
                  className="absolute right-0 top-0 bottom-0 pointer-events-none transition-all duration-200"
                  style={{ 
                    width: `${widthPct}%`, 
                    backgroundColor: 'rgba(0, 200, 83, 0.15)' 
                  }}
                />
                <span className="text-[#00C853] font-bold w-1/3 text-left relative z-10">{parseFloat(bid.price).toFixed(2)}</span>
                <span className="text-[#E0E0E0] w-1/3 text-right relative z-10">{bid.qty.toFixed(4)}</span>
                <span className="text-[#888888] w-1/3 text-right relative z-10">{bid.cumulative.toFixed(4)}</span>
              </div>
            );
          })}
        </div>
      </div>
    </div>
  );
};

export default OrderBookTable;
