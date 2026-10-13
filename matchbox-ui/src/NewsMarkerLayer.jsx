import { useEffect, useState, useRef, useCallback } from 'react';

/**
 * NewsMarkerLayer
 * 
 * Manages news event markers on a TradingView Lightweight Chart series.
 * Listens for news events via a callback (from parent WebSocket), and places
 * colored arrow markers on the corresponding candle.
 * 
 * Props:
 *   series        - The main chart series ref (areaSeries or candlestickSeries)
 *   currentSymbol - e.g. "BTC/USDT"
 *   chartRef      - The chart instance ref (for coordinate conversion on click)
 */

// ─── Marker State Manager (module-level for persistence across re-renders) ───
let _newsMarkers = [];

export function addNewsMarker(event, currentSymbol) {
  // Only add markers for the currently viewed symbol
  if (event.symbol && event.symbol !== currentSymbol) return null;

  const isBullish = event.sentiment === 'BULLISH';
  const isHighImpact = event.impact >= 4;

  const marker = {
    time: event.timestamp,
    position: isBullish ? 'aboveBar' : 'belowBar',
    color: isBullish ? '#00C853' : '#FF1744',
    shape: isBullish ? 'arrowUp' : 'arrowDown',
    text: isHighImpact ? '⚡' : (isBullish ? '▲' : '▼'),
    // Store metadata for popover
    _meta: {
      id: event.id,
      title: event.title,
      summary: event.summary || '',
      sentiment: event.sentiment,
      impact: event.impact,
      timestamp: event.timestamp,
    },
  };

  // Prevent duplicate markers for same news ID
  if (_newsMarkers.find(m => m._meta.id === event.id)) return null;

  _newsMarkers.push(marker);

  // Sort by time (lightweight-charts requires sorted markers)
  _newsMarkers.sort((a, b) => a.time - b.time);

  return marker;
}

export function getNewsMarkers() {
  return _newsMarkers;
}

export function clearNewsMarkers() {
  _newsMarkers = [];
}

export function applyMarkersToSeries(series) {
  if (!series) return;
  // lightweight-charts setMarkers expects markers without custom _meta fields
  // but it actually ignores unknown fields, so we can pass them through
  try {
    series.setMarkers([..._newsMarkers]);
  } catch (e) {
    // Series might have been removed
  }
}

// ─── Popover Component ───
export function NewsPopover({ popover, onClose }) {
  if (!popover) return null;

  const { x, y, meta } = popover;
  const isBullish = meta.sentiment === 'BULLISH';

  // Compute popover position (keep within viewport)
  const popW = 320;
  const popH = 180;
  let left = x - popW / 2;
  let top = y - popH - 16;
  if (left < 8) left = 8;
  if (left + popW > window.innerWidth - 8) left = window.innerWidth - popW - 8;
  if (top < 8) top = y + 24; // flip below if clipping top

  const borderColor = isBullish ? '#00C853' : '#FF1744';
  const bgGlow = isBullish ? 'rgba(0,200,83,0.08)' : 'rgba(255,23,68,0.08)';

  const date = new Date(meta.timestamp * 1000);
  const timeStr = date.toLocaleTimeString('tr-TR', { hour: '2-digit', minute: '2-digit' });
  const dateStr = date.toLocaleDateString('tr-TR', { day: '2-digit', month: 'short' });

  return (
    <div
      className="fixed z-[100] pointer-events-auto"
      style={{ left, top, width: popW }}
      onClick={(e) => e.stopPropagation()}
    >
      <div
        className="rounded-xl border overflow-hidden backdrop-blur-xl shadow-2xl"
        style={{
          borderColor,
          backgroundColor: '#0A0A0A',
          boxShadow: `0 8px 32px -8px ${bgGlow}, 0 0 0 1px ${borderColor}40`,
        }}
      >
        {/* Header */}
        <div
          className="px-4 py-2.5 flex items-center justify-between"
          style={{ background: `linear-gradient(135deg, ${bgGlow}, transparent)` }}
        >
          <div className="flex items-center gap-2">
            <span
              className="text-[10px] font-black tracking-widest uppercase px-2 py-0.5 rounded-full border"
              style={{
                color: borderColor,
                borderColor: `${borderColor}50`,
                backgroundColor: `${borderColor}15`,
              }}
            >
              {meta.sentiment}
            </span>
            {meta.impact >= 4 && (
              <span className="text-sm" title={`Impact: ${meta.impact}/5`}>⚡</span>
            )}
            <div className="flex gap-0.5">
              {Array.from({ length: 5 }).map((_, i) => (
                <div
                  key={i}
                  className="w-1.5 h-1.5 rounded-full"
                  style={{
                    backgroundColor: i < meta.impact ? borderColor : '#333333',
                  }}
                />
              ))}
            </div>
          </div>
          <div className="flex items-center gap-2">
            <span className="text-[10px] text-[#888888] font-mono">{dateStr} {timeStr}</span>
            <button
              onClick={onClose}
              className="text-[#888888] hover:text-white transition-colors text-xs font-bold w-5 h-5 flex items-center justify-center rounded hover:bg-[#333333]"
            >
              ✕
            </button>
          </div>
        </div>

        {/* Body */}
        <div className="px-4 py-3">
          <h4 className="text-white text-sm font-bold leading-tight mb-2 line-clamp-2">
            {meta.title}
          </h4>
          {meta.summary && (
            <p className="text-[#888888] text-xs leading-relaxed line-clamp-3">
              {meta.summary}
            </p>
          )}
        </div>

        {/* Footer gradient line */}
        <div
          className="h-0.5"
          style={{
            background: `linear-gradient(90deg, transparent, ${borderColor}, transparent)`,
          }}
        />
      </div>
    </div>
  );
}

/**
 * useNewsMarkers hook
 * 
 * Call this in your main App component to manage news markers.
 * Returns { handleNewsEvent, popover, setPopover, setupChartClickHandler }
 */
export function useNewsMarkers(seriesRef, chartRef, chartContainerRef, currentSymbol) {
  const [popover, setPopover] = useState(null);
  const markersAppliedRef = useRef(false);

  // Handle incoming news event (call this from WS onmessage)
  const handleNewsEvent = useCallback((event) => {
    const marker = addNewsMarker(event, currentSymbol);
    if (marker && seriesRef?.current) {
      applyMarkersToSeries(seriesRef.current);
      markersAppliedRef.current = true;
    }
  }, [currentSymbol, seriesRef]);

  // Clear markers when symbol changes
  useEffect(() => {
    clearNewsMarkers();
    if (seriesRef?.current) {
      applyMarkersToSeries(seriesRef.current);
    }
    setPopover(null);
  }, [currentSymbol]);

  // Load existing news from REST API as initial markers
  useEffect(() => {
    const loadExistingNews = async () => {
      try {
        const apiUrl = import.meta.env.DEV
          ? `http://localhost:8080/api/v1/news`
          : `/api/v1/news`;
        const res = await fetch(apiUrl);
        if (!res.ok) return;
        const data = await res.json();
        if (!Array.isArray(data)) return;

        clearNewsMarkers();
        data.forEach(news => {
          if (!news.time && !news.timestamp) return;
          const ts = news.timestamp || Math.floor(new Date(news.time).getTime() / 1000);
          addNewsMarker({
            id: news.id,
            timestamp: ts,
            title: news.title,
            summary: news.summary || '',
            sentiment: news.sentiment,
            impact: news.impact || 1,
            symbol: null, // Allow all symbols for historical data
          }, currentSymbol);
        });

        if (seriesRef?.current) {
          applyMarkersToSeries(seriesRef.current);
        }
      } catch (e) {
        // Silent fail
      }
    };

    // Small delay to ensure chart series is ready
    const timer = setTimeout(loadExistingNews, 1500);
    return () => clearTimeout(timer);
  }, [currentSymbol]);

  // Setup click handler on the chart to show popover
  useEffect(() => {
    const chart = chartRef?.current;
    const container = chartContainerRef?.current;
    if (!chart || !container) return;

    const handleClick = (param) => {
      if (!param.time || !param.point) {
        setPopover(null);
        return;
      }

      // Find if there's a marker at this time
      const clickedTime = param.time;
      const marker = _newsMarkers.find(m => m.time === clickedTime);

      if (marker && marker._meta) {
        // Get screen coordinates from the container
        const rect = container.getBoundingClientRect();
        const screenX = rect.left + param.point.x;
        const screenY = rect.top + param.point.y;

        setPopover({
          x: screenX,
          y: screenY,
          meta: marker._meta,
        });
      } else {
        setPopover(null);
      }
    };

    chart.subscribeClick(handleClick);
    return () => {
      try {
        chart.unsubscribeClick(handleClick);
      } catch (e) {
        // Chart might already be removed
      }
    };
  }, [chartRef?.current, chartContainerRef?.current]);

  // Close popover on Escape or outside click
  useEffect(() => {
    if (!popover) return;

    const handleKey = (e) => {
      if (e.key === 'Escape') setPopover(null);
    };
    const handleOutsideClick = () => setPopover(null);

    window.addEventListener('keydown', handleKey);
    // Delay adding click listener to prevent immediate close
    const timer = setTimeout(() => {
      window.addEventListener('click', handleOutsideClick);
    }, 100);

    return () => {
      window.removeEventListener('keydown', handleKey);
      window.removeEventListener('click', handleOutsideClick);
      clearTimeout(timer);
    };
  }, [popover]);

  return { handleNewsEvent, popover, setPopover };
}
