import React, { useEffect, useRef, useState, useImperativeHandle, forwardRef } from 'react';
import { createChart } from 'lightweight-charts';

const TradingChart = forwardRef(({ currentSymbol }, ref) => {
  const chartContainerRef = useRef(null);
  const chartRef = useRef(null);
  const seriesRef = useRef(null);
  
  const [timeframe, setTimeframe] = useState('1m');
  const [isLoading, setIsLoading] = useState(false);

  // 1. Initialize Chart
  useImperativeHandle(ref, () => ({
    get series() {
      return seriesRef.current;
    },
    setTimeframe: (tf) => {
      setTimeframe(tf);
    },
    resetChart: () => {
      if (chartRef.current) {
        chartRef.current.timeScale().scrollToRealTime();
        chartRef.current.timeScale().fitContent();
      }
    }
  }), []);

  useEffect(() => {
    if (!chartContainerRef.current) return;

    // Create the chart instance
    const chart = createChart(chartContainerRef.current, {
      layout: {
        background: { type: 'solid', color: '#050505' },
        textColor: '#FFFFFF',
      },
      grid: {
        vertLines: { color: 'rgba(255, 255, 255, 0.04)' },
        horzLines: { color: 'rgba(255, 255, 255, 0.04)' },
      },
      crosshair: {
        mode: 1, // Normal crosshair
        vertLine: {
          color: 'rgba(255, 255, 255, 0.4)',
          width: 1,
          style: 3, // dashed
          labelBackgroundColor: '#222222',
        },
        horzLine: {
          color: 'rgba(255, 255, 255, 0.4)',
          width: 1,
          style: 3,
          labelBackgroundColor: '#222222',
        },
      },
      timeScale: {
        borderColor: '#1A1A1A',
        timeVisible: true,
        secondsVisible: false,
      },
      rightPriceScale: {
        borderColor: '#1A1A1A',
      },
    });

    // Add Candlestick Series
    const candlestickSeries = chart.addCandlestickSeries({
      upColor: '#00C853', // Neon Green
      downColor: '#FF1744', // Red
      borderVisible: false,
      wickUpColor: '#00C853',
      wickDownColor: '#FF1744',
    });

    chartRef.current = chart;
    seriesRef.current = candlestickSeries;

    if (ref) {
      if (typeof ref === 'function') {
        ref({ chart, series: candlestickSeries });
      } else {
        ref.current = { chart, series: candlestickSeries };
      }
    }

    // Zero-Memory-Leak Resize Observer
    const handleResize = () => {
      if (chartContainerRef.current && chartRef.current) {
        chartRef.current.applyOptions({
          width: chartContainerRef.current.clientWidth,
          height: chartContainerRef.current.clientHeight,
        });
      }
    };

    const resizeObserver = new ResizeObserver(handleResize);
    resizeObserver.observe(chartContainerRef.current);
    
    // Trigger initial resize
    handleResize();

    // Cleanup phase
    return () => {
      resizeObserver.disconnect();
      if (chartRef.current) {
        chartRef.current.remove();
        chartRef.current = null;
        seriesRef.current = null;
      }
    };
  }, []); // Empty dependency array ensures chart is created only once

  // 2. Fetch and apply data on symbol/timeframe change
  useEffect(() => {
    if (!seriesRef.current) return;

    let isMounted = true;
    
    const fetchKlines = async () => {
      setIsLoading(true);
      try {
        const interval = timeframe === '1H' ? '1h' : timeframe === '1D' ? '1d' : timeframe;
        const apiUrl = import.meta.env.DEV 
          ? `http://localhost:8080/api/v1/klines?symbol=${currentSymbol}&interval=${interval}` 
          : `/api/v1/klines?symbol=${currentSymbol}&interval=${interval}`;
          
        const res = await fetch(apiUrl);
        if (!res.ok) throw new Error('Failed to fetch klines');
        
        const data = await res.json();
        
        if (isMounted) {
          const formattedData = data.map(k => ({
            time: k.time,
            open: parseFloat(k.open),
            high: parseFloat(k.high),
            low: parseFloat(k.low),
            close: parseFloat(k.close)
          }));
          
          seriesRef.current.setData(formattedData);
          chartRef.current.timeScale().fitContent(); // Auto fit to view
        }
      } catch (err) {
        console.error('Kline Error:', err);
      } finally {
        if (isMounted) setIsLoading(false);
      }
    };

    fetchKlines();

    return () => {
      isMounted = false; // Prevent state updates if unmounted before fetch completes
    };
  }, [currentSymbol, timeframe]);

  // 3. Optional: Live WebSocket Updates (If you want to push real-time ticks to the candle)
  // You can export a ref or use Zustand here to call seriesRef.current.update(newCandle)
  
  return (
    <div className="relative flex-1 w-full h-full flex flex-col bg-[#050505]">
      {/* Timeframe Controls Overlaid */}
      <div className="absolute top-4 left-4 z-10 flex gap-2 bg-[#0A0A0A]/80 backdrop-blur p-1 rounded-lg border border-[#1A1A1A]">
        {['1m', '15m', '1H', '1D'].map((tf) => (
          <button
            key={tf}
            onClick={() => setTimeframe(tf)}
            disabled={isLoading}
            className={`px-3 py-1 text-xs font-bold rounded-md transition-all duration-200 ${
              timeframe === tf
                ? 'bg-[#222222] text-[#00C853] shadow border border-[#333333]'
                : 'text-[#888888] hover:text-white hover:bg-[#1A1A1A]'
            } ${isLoading ? 'opacity-50 cursor-not-allowed' : ''}`}
          >
            {tf}
          </button>
        ))}
      </div>

      {/* Loading Overlay */}
      {isLoading && (
        <div className="absolute inset-0 z-20 flex items-center justify-center bg-[#050505]/50 backdrop-blur-sm">
          <div className="flex flex-col items-center gap-3">
            <div className="w-6 h-6 border-2 border-[#00C853] border-t-transparent rounded-full animate-spin"></div>
            <span className="text-[#00C853] text-xs font-mono font-bold tracking-widest">YÜKLENİYOR...</span>
          </div>
        </div>
      )}

      {/* Chart Container */}
      <div 
        ref={chartContainerRef} 
        className="flex-1 w-full h-full min-h-[300px]"
      />
    </div>
  );
});

export default TradingChart;
