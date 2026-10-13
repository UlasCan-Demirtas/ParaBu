import React, { useState, useEffect } from 'react';
import { Activity } from 'lucide-react';

export const NetworkLatencyWidget = ({ ping, connected }) => {
  const [history, setHistory] = useState(Array(30).fill(0));

  useEffect(() => {
    setHistory(prev => {
      const next = [...prev.slice(1), connected ? (ping || 0) : -1];
      return next;
    });
  }, [ping, connected]);

  const latestPing = history[history.length - 1];
  
  // Calculate SVG points for sparkline
  // -1 means drop, max expected ping ~300ms for scale
  const points = history.map((p, i) => {
    const x = (i / 29) * 40; // width 40
    let y = 16; // height 16
    
    if (p === -1) {
      y = 16; // full drop
    } else {
      // Map 0-300ms to 16-0 height (clamp)
      const scaled = Math.min(Math.max(p, 0), 300);
      y = 16 - (scaled / 300) * 16;
    }
    return `${x},${y}`;
  }).join(' ');

  // Determine color based on latest status
  let colorClass = 'text-[#00C853]'; // Green (< 50ms implicitly handled by else)
  let strokeColor = '#00C853';
  let containerClass = 'bg-[#111111] border-[#1A1A1A]';

  if (!connected || latestPing === -1) {
    colorClass = 'text-[#FF3B30]'; // Red Drop
    strokeColor = '#FF3B30';
    containerClass = 'bg-[#FF3B30]/10 border-[#FF3B30]/30 shadow-[0_0_10px_rgba(255,59,48,0.2)] animate-pulse';
  } else if (latestPing > 150) {
    colorClass = 'text-[#FF9800]'; // Yellow/Orange
    strokeColor = '#FF9800';
  }

  return (
    <div className={`flex items-center gap-2.5 px-3 py-1 rounded-full border transition-all ${containerClass}`} title="Network RTT Latency">
      <Activity className={`w-3.5 h-3.5 ${colorClass}`} />
      
      <div className="flex flex-col gap-0.5 justify-center">
        <div className="flex items-center gap-1.5">
          <span className="text-[#888888] text-[9px] font-bold uppercase tracking-widest leading-none mt-0.5">Ping</span>
          <span className={`font-mono text-xs font-black leading-none ${colorClass}`}>
            {!connected || latestPing === -1 ? 'DROP' : `${latestPing}ms`}
          </span>
        </div>
        
        <div className="w-12 h-3.5 flex items-end opacity-80">
          <svg viewBox="0 0 40 16" className="w-full h-full overflow-visible">
            <polyline
              points={points}
              fill="none"
              stroke={strokeColor}
              strokeWidth="1.5"
              strokeLinecap="round"
              strokeLinejoin="round"
              className="transition-all duration-300"
            />
          </svg>
        </div>
      </div>
    </div>
  );
};
