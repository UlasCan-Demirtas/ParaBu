import React, { useRef, useEffect, useState } from 'react';
import { Maximize2, Minimize2 } from 'lucide-react';

function getTreemapLayout(items, width, height) {
  const sorted = [...items].sort((a, b) => b.volume - a.volume);
  let result = [];
  
  function sliceAndDice(list, x, y, w, h) {
    if (list.length === 0) return;
    if (list.length === 1) {
      result.push({ ...list[0], x, y, w, h });
      return;
    }
    
    const total = list.reduce((sum, item) => sum + item.volume, 0);
    if (total === 0) return; // Prevent division by zero
    
    let halfSum = 0;
    let splitIndex = 0;
    for (let i = 0; i < list.length; i++) {
      halfSum += list[i].volume;
      if (halfSum >= total / 2) {
        splitIndex = i;
        break;
      }
    }
    
    const list1 = list.slice(0, splitIndex + 1);
    const list2 = list.slice(splitIndex + 1);
    const sum1 = list1.reduce((s, i) => s + i.volume, 0);
    const ratio = sum1 / total;
    
    if (w > h) { // split horizontally
      const w1 = w * ratio;
      sliceAndDice(list1, x, y, w1, h);
      sliceAndDice(list2, x + w1, y, w - w1, h);
    } else { // split vertically
      const h1 = h * ratio;
      sliceAndDice(list1, x, y, w, h1);
      sliceAndDice(list2, x, y + h1, w, h - h1);
    }
  }
  
  sliceAndDice(sorted, 0, 0, width, height);
  return result;
}

function getSentimentColor(score) {
  let s = Math.max(-1, Math.min(1, score));
  if (s === 0) return '#333333';
  if (s > 0) {
    // Gray (51,51,51) to Emerald Green (0,200,83)
    const r = Math.round(51 + s * (0 - 51));
    const g = Math.round(51 + s * (200 - 51));
    const b = Math.round(51 + s * (83 - 51));
    return `rgb(${r}, ${g}, ${b})`;
  } else {
    // Gray (51,51,51) to Dark Red (255,59,48)
    const abs = Math.abs(s);
    const r = Math.round(51 + abs * (255 - 51));
    const g = Math.round(51 + abs * (59 - 51));
    const b = Math.round(51 + abs * (48 - 51));
    return `rgb(${r}, ${g}, ${b})`;
  }
}

export function SentimentHeatmap({ data, onSelectSymbol, currentSymbol }) {
  const containerRef = useRef(null);
  const [dimensions, setDimensions] = useState({ width: 0, height: 0 });
  const [isMaximized, setIsMaximized] = useState(false);

  useEffect(() => {
    if (!containerRef.current) return;
    const observer = new ResizeObserver((entries) => {
      for (let entry of entries) {
        setDimensions({
          width: entry.contentRect.width,
          height: entry.contentRect.height
        });
      }
    });
    observer.observe(containerRef.current);
    return () => observer.disconnect();
  }, [isMaximized]);

  const layout = dimensions.width > 0 && dimensions.height > 0
    ? getTreemapLayout(data, dimensions.width, dimensions.height)
    : [];

  const content = (
    <div className={`flex flex-col h-full bg-[#0A0A0A] border border-[#1A1A1A] rounded-xl overflow-hidden ${isMaximized ? 'shadow-[0_0_50px_rgba(0,0,0,0.8)]' : ''}`}>
      <div className="p-3 border-b border-[#1A1A1A] bg-[#050505] flex justify-between items-center">
        <h3 className="font-bold text-white text-xs flex items-center gap-2">
          <button onClick={() => setIsMaximized(!isMaximized)} className="text-[#00C853] hover:text-white transition-colors cursor-pointer p-1 -m-1 rounded hover:bg-[#1A1A1A]" title={isMaximized ? "Küçült" : "Tam Ekran Yap"}>
            {isMaximized ? <Minimize2 className="w-3.5 h-3.5" /> : <Maximize2 className="w-3.5 h-3.5" />}
          </button>
          Sentiment Heatmap
        </h3>
        <span className="text-[10px] text-[#888888]">Boyut: Hacim | Renk: Duygu</span>
      </div>
      
      <div className="flex-1 relative bg-[#000000]" ref={containerRef}>
        {layout.length === 0 ? (
          <div className="absolute inset-0 flex items-center justify-center text-[#555555] text-xs">Yükleniyor...</div>
        ) : (
          layout.map(item => {
            const isSelected = item.symbol === currentSymbol;
            const bgColor = getSentimentColor(item.sentiment);
            return (
              <div
                key={item.symbol}
                onClick={() => onSelectSymbol(item.symbol)}
                className={`absolute cursor-pointer transition-all duration-300 overflow-hidden flex flex-col items-center justify-center p-1 border ${isSelected ? 'border-white z-10 shadow-[0_0_15px_rgba(255,255,255,0.2)]' : 'border-[#000000] hover:border-[#888888] hover:z-10'}`}
                style={{
                  left: item.x,
                  top: item.y,
                  width: item.w,
                  height: item.h,
                  backgroundColor: bgColor,
                }}
              >
                <span className="text-white font-bold tracking-wider drop-shadow-md leading-none text-center" style={{ fontSize: Math.min(18, Math.max(9, Math.min(item.w, item.h) / 3.5)) + 'px' }}>
                  {item.symbol.replace('USDT', '')}
                </span>
                
                {item.h > 70 && item.w > 60 && (
                  <span className="text-white/80 font-mono drop-shadow-md mt-1 leading-none" style={{ fontSize: Math.min(11, Math.max(8, item.h / 8)) + 'px' }}>
                    {item.sentiment > 0 ? '+' : ''}{item.sentiment.toFixed(2)}
                  </span>
                )}
                
                {item.h > 95 && item.w > 70 && item.volume > 0 && (
                  <span className="text-white/50 font-mono drop-shadow-md mt-1 leading-none" style={{ fontSize: Math.min(10, Math.max(7, item.h / 10)) + 'px' }}>
                    ${(item.volume / 1000000).toFixed(1)}M
                  </span>
                )}
              </div>
            );
          })
        )}
      </div>
    </div>
  );

  if (isMaximized) {
    return (
      <div className="fixed inset-0 z-[100] bg-black/90 backdrop-blur-sm p-4 md:p-12 flex flex-col">
        {content}
      </div>
    );
  }

  return content;
}
