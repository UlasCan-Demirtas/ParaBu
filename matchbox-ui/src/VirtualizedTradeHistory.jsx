import React, { useRef, useEffect, useState } from 'react';
import { useVirtualizer } from '@tanstack/react-virtual';

const TradeRow = ({ trade, index, style }) => {
  const isBuy = trade.type === 'buy';
  const color = isBuy ? '#00C853' : '#FF1744';
  const textColor = isBuy ? 'text-[#00C853]' : 'text-[#FF3B30]';
  const amountNum = parseFloat(trade.amount);
  
  // Is this a "whale" trade? (amount > 1 BTC)
  const isLarge = amountNum >= 1.0;
  
  // We use keyframe animations defined in App.css or inline
  let animationClass = 'animate-[slide-down_0.2s_ease-out_forwards]';
  if (isLarge && index === 0) { // flash only when it arrives at the top
    animationClass += isBuy ? ' animate-[flash-bg-green_0.8s_ease-out_forwards]' : ' animate-[flash-bg-red_0.8s_ease-out_forwards]';
  }

  return (
    <div 
      style={{
        ...style,
        position: 'absolute',
        top: 0,
        left: 0,
        width: '100%',
        transform: `translateY(${style.top}px)`
      }}
      className={`flex justify-between items-center px-3 py-1.5 hover:bg-[#1A1A1A] transition-colors border-b border-[#1A1A1A]/50 ${animationClass}`}
    >
      <span className="text-[#888888] font-mono text-[10px] w-16">
        {trade.time}
      </span>
      <span className={`font-mono text-xs font-bold w-24 text-right ${textColor}`}>
        {trade.price}
      </span>
      <span className="text-white font-mono text-xs w-20 text-right">
        {trade.amount}
      </span>
    </div>
  );
};

export default function VirtualizedTradeHistory({ trades }) {
  const parentRef = useRef(null);

  // Maintain max 30 items visible (handled inherently by useVirtualizer with container height)
  // The container will be e.g. 30 * rowHeight pixels, but we can just let flex grow it, and react-virtual will render what's visible.
  
  const rowVirtualizer = useVirtualizer({
    count: trades.length,
    getScrollElement: () => parentRef.current,
    estimateSize: () => 32, // approximately 32px height per row
    overscan: 5, // buffer rows outside visible area
  });

  return (
    <div className="flex flex-col h-full bg-[#0A0A0A] overflow-hidden relative">
      {/* Header */}
      <div className="flex justify-between items-center px-3 py-2 bg-[#111111] border-b border-[#1A1A1A]">
        <span className="text-[#555555] font-bold text-[10px] uppercase w-16">Zaman</span>
        <span className="text-[#555555] font-bold text-[10px] uppercase w-24 text-right">Fiyat (USDT)</span>
        <span className="text-[#555555] font-bold text-[10px] uppercase w-20 text-right">Miktar (BTC)</span>
      </div>

      {/* Virtualized List Container */}
      <div 
        ref={parentRef} 
        className="flex-1 overflow-auto custom-scrollbar relative"
        style={{ height: '400px' }} // Height can be controlled by parent's flex layout too, we let flex-1 handle it but keep overflow-auto
      >
        <div
          style={{
            height: `${rowVirtualizer.getTotalSize()}px`,
            width: '100%',
            position: 'relative',
          }}
        >
          {rowVirtualizer.getVirtualItems().map((virtualRow) => {
            const trade = trades[virtualRow.index];
            return (
              <TradeRow 
                key={trade.id || virtualRow.index} // fallback to index if no id
                trade={trade} 
                index={virtualRow.index}
                style={{
                  height: `${virtualRow.size}px`,
                  top: virtualRow.start,
                }} 
              />
            );
          })}
        </div>
      </div>
    </div>
  );
}
