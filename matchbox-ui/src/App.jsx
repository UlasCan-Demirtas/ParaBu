import React, { useEffect, useState, useRef, useCallback } from 'react';
import { createChart, ColorType } from 'lightweight-charts';
import { Activity, Server, Zap, AlignLeft, Wallet, Globe, ArrowUpRight, ArrowDownRight, X, TrendingUp, TrendingDown, Minus, ExternalLink, Flame, Eye, EyeOff, Keyboard, Volume2, VolumeX, BarChart3 } from 'lucide-react';
import { useNewsMarkers, NewsPopover } from './NewsMarkerLayer';
import { useWhaleWallDetector } from './hooks/useWhaleWallDetector';
import { SentimentHeatmap } from './SentimentHeatmap';
import { SessionOverlapBar } from './SessionOverlapBar';
import { useSentimentDivergence } from './hooks/useSentimentDivergence';
import { NewsContentHighlighter } from './NewsContentHighlighter';
import { NetworkLatencyWidget } from './NetworkLatencyWidget';
import RightUtilityPanel from './RightUtilityPanel';
import TradingChart from './TradingChart';
import OrderForm from './OrderForm';
import ConnectionStatusBadge from './ConnectionStatusBadge';
import TradingToastManager from './TradingToastManager';
import { useTradingHotkeys } from './hooks/useTradingHotkeys';
import { showToast } from './toastSystem';
import { VirtualOrdersList } from './VirtualOrdersList';
import { Panel, PanelGroup, PanelResizeHandle } from 'react-resizable-panels';
import './App.css';

const CustomResizeHandle = () => {
  return (
    <PanelResizeHandle className="w-3 relative group flex justify-center items-center cursor-col-resize transition-colors">
      {/* Invisible larger hit area for easier grabbing */}
      <div className="absolute inset-y-0 -left-2 -right-2 z-10"></div>
      <div className="w-0.5 h-16 bg-[#222222] group-hover:bg-[#00C853] group-hover:shadow-[0_0_12px_rgba(0,200,83,0.8)] rounded-full transition-all duration-300 ease-out z-20"></div>
    </PanelResizeHandle>
  );
};

import { soundEngine } from './SoundEngine';

const QuickNewsTradeBar = ({ news, authToken, onSuccess }) => {
  const [loading, setLoading] = useState(null);
  
  const symbol = news.symbol || 'BTCUSDT';
  const baseAsset = symbol.replace('USDT', '');
  
  // Dynamic default risk size
  const defaultQty = baseAsset === 'BTC' ? '0.01' : baseAsset === 'ETH' ? '0.1' : baseAsset === 'XRP' ? '100' : '10';

  const executeTrade = async (e, side) => {
    e.stopPropagation(); // prevent popover from opening when clicking button
    setLoading(side);

    try {
      const apiUrl = import.meta.env.DEV ? 'http://localhost:8080/api/v1/order' : '/api/v1/order';
      const res = await fetch(apiUrl, {
        method: 'POST',
        headers: { 
          'Content-Type': 'application/json',
          'Authorization': `Bearer ${authToken}`
        },
        body: JSON.stringify({
          symbol: symbol,
          side: side,
          type: 'MARKET',
          price: '0',
          quantity: defaultQty
        })
      });
      
      const data = await res.json();
      if (!res.ok) throw new Error(data.error || 'Failed');
      
      soundEngine.playMatchChime();
      showToast(`${defaultQty} ${baseAsset} @ Piyasa`, 'success', 'Emir Eşleşti');
      if (onSuccess) onSuccess();
    } catch(err) {
      showToast(err.message || 'İşlem başarısız', 'error', 'İşlem Reddedildi');
    } finally {
      setLoading(null);
    }
  };

  return (
    <div className="mt-3 flex gap-2 items-center relative">
      <button 
        onClick={(e) => executeTrade(e, 'BUY')}
        disabled={loading}
        className="flex-1 bg-[#00C853]/10 hover:bg-[#00C853]/20 border border-[#00C853]/30 text-[#00C853] text-[10px] font-bold py-1.5 rounded transition-colors flex justify-center items-center gap-1 disabled:opacity-50"
      >
        {loading === 'BUY' ? <span className="animate-spin inline-block w-3 h-3 border-2 border-[#00C853] border-t-transparent rounded-full"></span> : '⚡'} 
        Hızlı Al ({defaultQty} {baseAsset})
      </button>
      <button 
        onClick={(e) => executeTrade(e, 'SELL')}
        disabled={loading}
        className="flex-1 bg-[#FF3B30]/10 hover:bg-[#FF3B30]/20 border border-[#FF3B30]/30 text-[#FF3B30] text-[10px] font-bold py-1.5 rounded transition-colors flex justify-center items-center gap-1 disabled:opacity-50"
      >
        {loading === 'SELL' ? <span className="animate-spin inline-block w-3 h-3 border-2 border-[#FF3B30] border-t-transparent rounded-full"></span> : '⚡'} 
        Hızlı Sat ({defaultQty} {baseAsset})
      </button>

      {/* Micro Toast */}
      {/* Toast logic removed from QuickNewsTradeBar */}
    </div>
  );
};

function App() {
  const [bids, setBids] = useState([]);
  const [asks, setAsks] = useState([]);
  const [currentPrice, setCurrentPrice] = useState(0);
  const [prevPrice, setPrevPrice] = useState(0);
  const [spread, setSpread] = useState(0);
  const [wsStatus, setWsStatus] = useState('CONNECTING');
  const [time, setTime] = useState(new Date());
  const tradingChartRef = useRef(null);
  const depthBufferRef = useRef([]);
  const rafRef = useRef(null);

  // Form State
  const [orderSide, setOrderSide] = useState('BUY');
  const [orderType, setOrderType] = useState('LIMIT');
  const [price, setPrice] = useState('');
  const [quantity, setQuantity] = useState('');

  // Active User Orders
  const [activeOrders, setActiveOrders] = useState([]);
  
  // Right Panel Tabs
  const [rightPanelTab, setRightPanelTab] = useState('ORDER_BOOK');
  const [recentTrades, setRecentTrades] = useState([]);

  // AI News Drawer State
  const [isNewsOpen, setIsNewsOpen] = useState(false);
  const [newsFeed, setNewsFeed] = useState([]);

  // ─── FAZ 1 NEW STATE ───
  const [indicators, setIndicators] = useState({
    sma20: true, ema9: true, ema21: true, ema50: false, ema200: false, bb: false,
    rsi: true, macd: true,
  });
  const [showIndicatorMenu, setShowIndicatorMenu] = useState(false);
  const [soundEnabled, setSoundEnabled] = useState(true);
  const lastThudTimeRef = useRef(0);

  useEffect(() => {
    soundEngine.setEnabled(soundEnabled);
  }, [soundEnabled]);
  const [wsPing, setWsPing] = useState(0);
  const wsRef = useRef(null);
  const pingIntervalRef = useRef(null);

  const fetchNews = async () => {
    try {
      const apiUrl = import.meta.env.DEV ? 'http://localhost:8080/api/v1/news' : '/api/v1/news';
      const res = await fetch(apiUrl);
      const data = await res.json();
      setNewsFeed(data || []);
    } catch(err) {
      console.error('Failed to fetch news', err);
    }
  };

  useEffect(() => {
    fetchNews();
    const interval = setInterval(fetchNews, 30000);
    return () => clearInterval(interval);
  }, []);

  const [timeframe, setTimeframe] = useState('1m');
  const [currentSymbol, setCurrentSymbol] = useState('BTC/USDT');
  const [tickers, setTickers] = useState([]);

  // News Markers integration
  const { handleNewsEvent, popover, setPopover } = useNewsMarkers(null, null, null, currentSymbol);

  // Real Balances from Backend Ledger
  const [balances, setBalances] = useState({
    usdt: 0, btc: 0, eth: 0, xrp: 0,
    usdtLocked: 0, btcLocked: 0, ethLocked: 0, xrpLocked: 0,
  });

  const [authToken, setAuthToken] = useState(null);
  const workerRef = useRef(null);
  const [latestRSI, setLatestRSI] = useState(null);

  // Initialize Web Worker for TA
  useEffect(() => {
    // We removed custom indicators from main app since we're rendering TradingChart now
    // Future: Connect worker directly to TradingChart if needed
  }, []);

  // Mock Authentication
  useEffect(() => {
    const login = async () => {
      try {
        const apiUrl = import.meta.env.DEV ? 'http://localhost:8080/api/v1/auth/mock-login?user_id=user-1' : '/api/v1/auth/mock-login?user_id=user-1';
        const res = await fetch(apiUrl, { method: 'POST' });
        const data = await res.json();
        if (data.access_token) {
          setAuthToken(data.access_token);
        }
      } catch(err) {
        console.error('Failed to mock login', err);
      }
    };
    login();
  }, []);

  const fetchBalances = async () => {
    if (!authToken) return;
    try {
      const apiUrl = import.meta.env.DEV ? 'http://localhost:8080/api/v1/balance?user_id=user-1' : '/api/v1/balance?user_id=user-1';
      const res = await fetch(apiUrl, {
        headers: {
          'Authorization': `Bearer ${authToken}`
        }
      });
      const data = await res.json();
      setBalances({
        usdt: parseFloat(data.USDT?.available || 0),
        usdtLocked: parseFloat(data.USDT?.locked || 0),
        btc: parseFloat(data.BTC?.available || 0),
        btcLocked: parseFloat(data.BTC?.locked || 0),
        eth: parseFloat(data.ETH?.available || 0),
        ethLocked: parseFloat(data.ETH?.locked || 0),
        xrp: parseFloat(data.XRP?.available || 0),
        xrpLocked: parseFloat(data.XRP?.locked || 0),
      });
    } catch(err) {
      console.error('Failed to fetch balances', err);
    }
  };

  useEffect(() => {
    if (!authToken) return;
    fetchBalances();
    const interval = setInterval(fetchBalances, 2000);
    return () => clearInterval(interval);
  }, [authToken]);

  // World Clocks Timer
  useEffect(() => {
    const timer = setInterval(() => setTime(new Date()), 1000);
    return () => clearInterval(timer);
  }, []);

  const getTime = (timeZone) => {
    return time.toLocaleTimeString('en-US', { timeZone, hour12: false, hour: '2-digit', minute: '2-digit', second: '2-digit' });
  };

  // ─── Trading Hotkeys ───
  useTradingHotkeys({
    onBuyFocus: () => {
      const id = orderType === 'LIMIT' ? 'price-BUY' : 'quantity-BUY';
      document.getElementById(id)?.focus();
    },
    onSellFocus: () => {
      const id = orderType === 'LIMIT' ? 'price-SELL' : 'quantity-SELL';
      document.getElementById(id)?.focus();
    },
    onEscape: () => {
      setIsNewsOpen(false);
      setPopover(null);
    },
    onTimeframeChange: (tf) => {
      tradingChartRef.current?.setTimeframe?.(tf);
    },
    onResetChart: () => {
      tradingChartRef.current?.resetChart?.();
    }
  });

  // ─── On-Chart Order Lines ───
  useEffect(() => {
    // Re-evaluate order lines logic if needed by delegating to TradingChart
  }, [activeOrders]);

  // ─── WebSocket Connection ───
  useEffect(() => {
    const wsUrl = import.meta.env.DEV ? 'ws://localhost:8080/ws' : `ws://${window.location.host}/ws`;
    const ws = new WebSocket(wsUrl);
    wsRef.current = ws;


    ws.onopen = () => {
      setWsStatus('OPEN');
      ws.send(JSON.stringify({
        action: "subscribe",
        topics: [`depth:${currentSymbol}`, `trade:${currentSymbol}`, `tickers`, `!news@sentiment`, `deposit:user-1`]
      }));

      // Ping measurement
      pingIntervalRef.current = setInterval(() => {
        if (ws.readyState === WebSocket.OPEN) {
          const start = performance.now();
          ws.send(JSON.stringify({ action: "ping" }));
          // Estimate round-trip based on next message
          ws._pingStart = start;
        }
      }, 2000);
    };
    
    ws.onclose = () => {
      setWsStatus('CLOSED');
      if (pingIntervalRef.current) clearInterval(pingIntervalRef.current);
    };

    let priceHistory = [];
    const currentPriceRef = { current: 0 }; // Local ref to avoid adding to dependencies

    // ─── BufferedChartUpdater (RAF Loop) ───
    const processBuffer = () => {
      if (depthBufferRef.current.length > 0) {
        // Take the latest payload to skip intermediate high-frequency ticks
        const latestPayload = depthBufferRef.current[depthBufferRef.current.length - 1];
        depthBufferRef.current = []; // clear buffer

        const newAsks = latestPayload.asks.slice(0, 20).reverse();
        const newBids = latestPayload.bids.slice(0, 20);
        setAsks(newAsks);
        setBids(newBids);

        const bestAsk = newAsks.length > 0 ? parseFloat(newAsks[newAsks.length - 1].price) : 0;
        const bestBid = newBids.length > 0 ? parseFloat(newBids[0].price) : 0;
        
        if (bestAsk > 0 && bestBid > 0) {
          setSpread((bestAsk - bestBid).toFixed(2));
        }
        
        const curr = bestAsk > 0 ? bestAsk : (bestBid > 0 ? bestBid : currentPriceRef.current);
        if (curr !== currentPriceRef.current) {
          // Check for sudden drop (>2%)
          if (currentPriceRef.current > 0 && curr < currentPriceRef.current * 0.98) {
            const nowMs = Date.now();
            if (nowMs - lastThudTimeRef.current > 5000) {
              soundEngine.playThud();
              lastThudTimeRef.current = nowMs;
            }
          }

          setPrevPrice(currentPriceRef.current);
          setCurrentPrice(curr);
          currentPriceRef.current = curr;
          
          const now = Math.floor(Date.now() / 1000);
          const lastPoint = priceHistory[priceHistory.length - 1];
          
          // Imperative update directly to TradingView series
          // Note: Candlestick requires {time, open, high, low, close} object, but a simple 
          // update can be passed just like area if we handle tick correctly, but wait, 
          // usually Candlestick expects { time, open, high, low, close }. 
          // We'll update the close price of the current candle.
          if (!lastPoint || now > lastPoint.time) {
            const point = { time: now, open: curr, high: curr, low: curr, close: curr };
            priceHistory.push(point);
            if (priceHistory.length > 100) priceHistory.shift();
            if (tradingChartRef.current?.series) tradingChartRef.current.series.update(point);
          } else if (now === lastPoint.time) {
            lastPoint.close = curr;
            lastPoint.high = Math.max(lastPoint.high, curr);
            lastPoint.low = Math.min(lastPoint.low, curr);
            if (tradingChartRef.current?.series) tradingChartRef.current.series.update(lastPoint);
          }
        }
      }
      rafRef.current = requestAnimationFrame(processBuffer);
    };
    rafRef.current = requestAnimationFrame(processBuffer);

    ws.onmessage = (event) => {
      // Measure ping
      if (ws._pingStart) {
        setWsPing(Math.round(performance.now() - ws._pingStart));
        ws._pingStart = null;
      }

      let msg;
      try {
        msg = JSON.parse(event.data);
      } catch(e) {
        return;
      }
      
      if (msg.topic === `depth:${currentSymbol}`) {
        // Buffer the payload instead of setting state directly
        depthBufferRef.current.push(msg.payload);
      } else if (msg.topic === `trade:${currentSymbol}`) {
        if (soundEnabled) soundEngine.playMatchChime();
        setRecentTrades(prev => {
          const newTrades = [msg.payload, ...prev];
          return newTrades.slice(0, 100); // Keep max 100 recent trades for virtual ring buffer
        });
      } else if (msg.topic === `tickers`) {
        setTickers(prevTickers => {
          const newMap = new Map(prevTickers.map(t => [t.symbol, t]));
          msg.payload.forEach(t => newMap.set(t.symbol, t));
          return Array.from(newMap.values());
        });
      } else if (msg.topic === `!news@sentiment`) {
        handleNewsEvent(msg.payload);
      } else if (msg.topic === `deposit:user-1`) {
        if (soundEnabled) soundEngine.playMatchChime(); // Or a specific coin sound
        showToast(`${msg.payload.amount} ${msg.payload.currency}`, 'success', '💰 Yatırma Başarılı');
      }
    };

    return () => {
      if (rafRef.current) cancelAnimationFrame(rafRef.current);
      if (pingIntervalRef.current) clearInterval(pingIntervalRef.current);
      if (ws.readyState === WebSocket.OPEN) {
        ws.send(JSON.stringify({
          action: "unsubscribe",
          topics: [`depth:${currentSymbol}`, `trade:${currentSymbol}`, `tickers`, `!news@sentiment`, `deposit:user-1`]
        }));
      }
      ws.close();
    };
  }, [currentSymbol, soundEnabled]);

  // ─── Keyboard Shortcuts ───
  useEffect(() => {
    const handler = (e) => {
      // Don't trigger when typing in inputs
      if (e.target.tagName === 'INPUT' || e.target.tagName === 'TEXTAREA' || e.target.tagName === 'SELECT') return;
      
      if (e.key === 'b' || e.key === 'B') {
        setOrderSide('BUY');
        document.getElementById('qty-input')?.focus();
      } else if (e.key === 's' || e.key === 'S') {
        setOrderSide('SELL');
        document.getElementById('qty-input')?.focus();
      } else if (e.key === 'c' || e.key === 'C') {
        // Cancel all orders
        activeOrders.forEach(o => cancelOrder(o.id, o.symbol));
      } else if (e.key === 'Escape') {
        setIsNewsOpen(false);
        setShowIndicatorMenu(false);
      }
    };
    window.addEventListener('keydown', handler);
    return () => window.removeEventListener('keydown', handler);
  }, [activeOrders]);

  const submitOrder = async (e) => {
    e.preventDefault();
    if (!price && orderType === 'LIMIT') return;
    if (!quantity) return;

    try {
      const apiUrl = import.meta.env.DEV ? 'http://localhost:8080/api/v1/order' : '/api/v1/order';
      const res = await fetch(apiUrl, {
        method: 'POST',
        headers: { 
          'Content-Type': 'application/json',
          'Authorization': `Bearer ${authToken}`
        },
        body: JSON.stringify({
          symbol: currentSymbol,
          side: orderSide,
          type: orderType,
          price: orderType === 'LIMIT' ? price : '0',
          quantity: quantity
        })
      });
      const data = await res.json();
      if (!res.ok) {
        throw new Error(data.error || 'Unknown error');
      }
      
      fetchBalances();

      if (orderType === 'LIMIT') {
        setActiveOrders(prev => [{
          id: data.order_id,
          symbol: currentSymbol,
          side: orderSide,
          price: price,
          qty: quantity,
          time: new Date().toLocaleTimeString()
        }, ...prev]);
      }
      
      if (soundEnabled) soundEngine.playMatchChime();
      if(orderType === 'MARKET') setQuantity('');
    } catch (err) {
      console.error('Failed to place order', err);
      alert('Order failed: ' + err.message);
    }
  };

  const cancelOrder = async (orderId, symbol) => {
    try {
      const apiUrl = import.meta.env.DEV 
        ? `http://localhost:8080/api/v1/order/${orderId}?symbol=${symbol}` 
        : `/api/v1/order/${orderId}?symbol=${symbol}`;
        
      const res = await fetch(apiUrl, { 
        method: 'DELETE',
        headers: { 'Authorization': `Bearer ${authToken}` }
      });
      if (!res.ok) {
        const data = await res.json();
        throw new Error(data.error || 'Cancel failed');
      }
      
      setActiveOrders(prev => prev.filter(o => o.id !== orderId));
      setTimeout(fetchBalances, 500);
      
    } catch (err) {
      console.error('Failed to cancel', err);
      alert('Cancel failed: ' + err.message);
    }
  };

  const updateOrderPrice = async (orderId, newPrice, symbol) => {
    // Optimistic UI update
    setActiveOrders(prev => prev.map(o => o.id === orderId ? { ...o, price: newPrice } : o));
    showToast(`$${newPrice}`, 'success', 'Fiyat Seçildi');
    
    // In a real application, you would make an API call to update the order price on the backend here.
    // e.g. await fetch(`/api/v1/order/${orderId}`, { method: 'PUT', body: JSON.stringify({ price: newPrice }) });
  };

  const calculateMaxVol = () => {
    let max = 0;
    asks.forEach(a => max = Math.max(max, parseFloat(a.quantity)));
    bids.forEach(b => max = Math.max(max, parseFloat(b.quantity)));
    return max;
  };

  const maxVol = calculateMaxVol();
  const whaleWalls = useWhaleWallDetector(asks, bids, currentPrice);
  const divergence = useSentimentDivergence(latestRSI, currentSymbol, newsFeed);

  useEffect(() => {
    Object.values(whaleWalls || {}).forEach(w => {
      if (w.proximityAlert) {
        const nowMs = Date.now();
        if (nowMs - lastThudTimeRef.current > 5000) {
          soundEngine.playThud();
          lastThudTimeRef.current = nowMs;
        }
      }
    });
  }, [whaleWalls]);
  const totalBalanceUSDT = (balances.usdt + (balances.btc * currentPrice)).toFixed(2);
  const priceDirection = currentPrice > prevPrice ? 'up' : currentPrice < prevPrice ? 'down' : 'neutral';
  const spreadPercent = currentPrice > 0 ? ((parseFloat(spread) / currentPrice) * 100).toFixed(4) : '0.00';

  const computedHeatmap = React.useMemo(() => {
    const data = [];
    tickers.forEach(t => {
      const vol = parseFloat(t.quoteVolume || t.volume || 0);
      const symbolNews = newsFeed.filter(n => n.symbol === t.symbol);
      let avgSentiment = 0;
      if (symbolNews.length > 0) {
        const sum = symbolNews.reduce((acc, n) => {
          let score = 0;
          if (n.sentiment === 'BULLISH') score = 1.0;
          else if (n.sentiment === 'BEARISH') score = -1.0;
          return acc + score * (n.impact || 1);
        }, 0);
        avgSentiment = sum / symbolNews.reduce((acc, n) => acc + (n.impact || 1), 0);
      } else {
        avgSentiment = (parseFloat(t.priceChangePercent) || 0) / 10;
      }
      data.push({ symbol: t.symbol, volume: vol, sentiment: avgSentiment });
    });
    const totalVolume = data.reduce((acc, curr) => acc + curr.volume, 0);
    if (data.length === 0 || totalVolume === 0) {
      // Mock data so the heatmap looks awesome even if backend has no volume yet
      return [
        { symbol: 'BTC/USDT', volume: 542000, sentiment: 0.8 },
        { symbol: 'ETH/USDT', volume: 310000, sentiment: 0.6 },
        { symbol: 'XRP/USDT', volume: 150000, sentiment: -0.4 },
        { symbol: 'SOL/USDT', volume: 220000, sentiment: 0.9 },
        { symbol: 'AVAX/USDT', volume: 90000, sentiment: 0.2 },
        { symbol: 'DOGE/USDT', volume: 120000, sentiment: -0.7 },
        { symbol: 'ADA/USDT', volume: 85000, sentiment: -0.2 },
        { symbol: 'LINK/USDT', volume: 70000, sentiment: 0.4 },
        { symbol: 'DOT/USDT', volume: 60000, sentiment: -0.5 },
        { symbol: 'MATIC/USDT', volume: 55000, sentiment: 0.1 }
      ];
    }
    return data;
  }, [tickers, newsFeed]);

  // ─── Indicator config for toggle menu ───
  const indicatorConfig = [
    { key: 'sma20', label: 'SMA 20', color: '#F5B041' },
    { key: 'ema9', label: 'EMA 9', color: '#3498DB' },
    { key: 'ema21', label: 'EMA 21', color: '#E74C3C' },
    { key: 'ema50', label: 'EMA 50', color: '#9B59B6' },
    { key: 'ema200', label: 'EMA 200', color: '#1ABC9C' },
    { key: 'bb', label: 'Bollinger Bands (20,2)', color: '#FFFFFF' },
    { key: 'rsi', label: 'RSI (14)', color: '#E67E22' },
    { key: 'macd', label: 'MACD (12,26,9)', color: '#3498DB' },
  ];

  return (
    <div className="min-h-screen bg-[#050505] text-white font-sans selection:bg-[#00C853] selection:text-black">

      {/* ═══ TICKER TAPE MARQUEE ═══ */}
      {tickers.length > 0 && (
        <div className="bg-[#000000] border-b border-[#111111] overflow-hidden h-7 flex items-center relative">
          <div className="absolute left-0 top-0 bottom-0 w-16 bg-gradient-to-r from-[#000000] to-transparent z-10"></div>
          <div className="absolute right-0 top-0 bottom-0 w-16 bg-gradient-to-l from-[#000000] to-transparent z-10"></div>
          <div className="flex animate-[marquee_60s_linear_infinite] gap-8 px-8 whitespace-nowrap">
            {[...tickers, ...tickers].map((t, i) => {
              const change = parseFloat(t.priceChangePercent || 0);
              const isUp = change >= 0;
              return (
                <button
                  key={`${t.symbol}-${i}`}
                  onClick={() => setCurrentSymbol(t.symbol)}
                  className="flex items-center gap-2 text-xs font-mono hover:opacity-80 transition-opacity shrink-0"
                >
                  <span className="text-white font-semibold">{t.symbol}</span>
                  <span className={isUp ? 'text-[#00C853]' : 'text-[#FF3B30]'}>
                    {parseFloat(t.lastPrice || 0).toFixed(2)}
                  </span>
                  <span className={`text-[10px] px-1 py-0.5 rounded ${isUp ? 'bg-[#00C853]/15 text-[#00C853]' : 'bg-[#FF3B30]/15 text-[#FF3B30]'}`}>
                    {isUp ? '+' : ''}{change.toFixed(2)}%
                  </span>
                </button>
              );
            })}
          </div>
        </div>
      )}
      
      {/* Premium Top Navigation */}
      <nav className="border-b border-[#1A1A1A] bg-[#0A0A0A] px-6 py-3 flex justify-between items-center sticky top-0 z-50">
        <div className="flex items-center gap-8">
          <div className="flex items-center gap-2">
            <div className="w-8 h-8 bg-[#00C853] flex items-center justify-center rounded-lg shadow-[0_0_15px_rgba(0,200,83,0.4)]">
              <Zap className="text-black w-5 h-5" fill="currentColor" />
            </div>
            <h1 className="text-xl font-bold tracking-tight text-white">
              PARABU<span className="text-[#00C853]">PRO</span>
            </h1>
          </div>

          {/* Session Overlap Widget */}
          <div className="hidden xl:flex items-center">
            <SessionOverlapBar />
          </div>
        </div>

          <div className="flex items-center gap-4 text-sm">
            {/* Keyboard Shortcuts Hint */}
            <div className="hidden lg:flex items-center gap-1.5 text-[10px] text-[#555555] font-mono">
              <Keyboard className="w-3.5 h-3.5" />
              <span>B</span>·<span>S</span>·<span>C</span>
            </div>

            {/* Sound Toggle */}
            <button
              onClick={() => {
                const next = !soundEnabled;
                setSoundEnabled(next);
                soundEngine.setEnabled(next);
                if (next) soundEngine.resume();
              }}
              className="text-[#555555] hover:text-white transition-colors p-1"
              title={soundEnabled ? 'Sesi Kapat' : 'Sesi Aç'}
            >
              {soundEnabled ? <Volume2 className="w-4 h-4" /> : <VolumeX className="w-4 h-4" />}
            </button>

            <button 
              onClick={() => setIsNewsOpen(true)}
              className="hidden md:flex items-center gap-2 bg-[#1A1A1A] hover:bg-[#222222] border border-[#333333] px-3 py-1.5 rounded-full transition-colors relative overflow-hidden group"
            >
              <div className="absolute inset-0 w-full h-full bg-gradient-to-r from-transparent via-white/5 to-transparent -translate-x-full group-hover:animate-[shimmer_1.5s_infinite]"></div>
              <Zap className="w-4 h-4 text-[#00C853]" fill="currentColor" />
              <span className="font-bold text-xs tracking-wider text-white">AI FEED</span>
              <span className="bg-[#00C853]/20 text-[#00C853] px-1.5 py-0.5 rounded text-[10px] font-bold">%68 BULL</span>
            </button>

            {/* Network Latency Sparkline */}
            <ConnectionStatusBadge status={wsStatus} ping={wsPing} />

            {/* Spread Warning */}
            {parseFloat(spreadPercent) > 0.5 && (
              <div className="flex items-center gap-1 bg-[#FF3B30]/10 border border-[#FF3B30]/30 px-2 py-1 rounded-full animate-pulse">
                <span className="text-[#FF3B30] text-[10px] font-bold">⚠ WIDE SPREAD {spreadPercent}%</span>
              </div>
            )}
          </div>
      </nav>

      {/* AI News Slide-over Drawer */}
      <div className={`fixed inset-y-0 right-0 w-[420px] bg-[#050505]/80 backdrop-blur-2xl border-l border-[#1A1A1A] z-[60] transform transition-transform duration-500 ease-[cubic-bezier(0.16,1,0.3,1)] shadow-[-20px_0_50px_rgba(0,0,0,0.9)] ${isNewsOpen ? 'translate-x-0' : 'translate-x-full'} flex flex-col`}>
        {/* Animated Background Gradients */}
        <div className="absolute top-0 right-0 w-[300px] h-[300px] bg-[#00C853]/10 rounded-full blur-[100px] pointer-events-none"></div>
        <div className="absolute bottom-0 left-0 w-[300px] h-[300px] bg-[#FF3B30]/5 rounded-full blur-[100px] pointer-events-none"></div>

        {/* Header */}
        <div className="relative p-6 border-b border-[#1A1A1A] flex justify-between items-center bg-gradient-to-b from-[#0A0A0A] to-transparent">
          <div className="flex items-center gap-4">
            <div className="w-10 h-10 bg-[#111111] rounded-xl border border-[#333333] flex items-center justify-center shadow-[0_0_15px_rgba(0,200,83,0.1)] relative group">
              <div className="absolute inset-0 bg-[#00C853]/20 blur-md rounded-xl group-hover:bg-[#00C853]/30 transition-colors"></div>
              <Zap className="w-5 h-5 text-[#00C853] relative z-10" fill="currentColor" />
            </div>
            <div>
              <h2 className="text-white font-extrabold text-xl tracking-tight bg-clip-text text-transparent bg-gradient-to-r from-white to-[#888888]">AI Market Feed</h2>
              <p className="text-[#00C853] text-[10px] uppercase font-mono tracking-[0.2em] mt-0.5 flex items-center gap-1">
                <span className="w-1.5 h-1.5 rounded-full bg-[#00C853] animate-pulse"></span>
                Live Intelligence
              </p>
            </div>
          </div>
          <button onClick={() => setIsNewsOpen(false)} className="text-[#888888] hover:text-white transition-all hover:rotate-90 bg-[#111111] p-2 rounded-lg border border-[#1A1A1A]">
            <X className="w-5 h-5" />
          </button>
        </div>

        {/* Global Market Sentiment Bar */}
        <div className="relative px-6 py-5 border-b border-[#1A1A1A] bg-[#050505]/50">
          <div className="flex justify-between text-[11px] font-bold tracking-wider mb-3">
            <span className="text-[#00C853] flex items-center gap-1"><TrendingUp className="w-3 h-3"/> %68 BULLISH</span>
            <span className="text-[#FF3B30] flex items-center gap-1">%32 BEARISH <TrendingDown className="w-3 h-3"/></span>
          </div>
          <div className="h-1.5 w-full bg-[#111111] rounded-full overflow-hidden flex shadow-[inset_0_1px_3px_rgba(0,0,0,1)]">
            <div className="h-full bg-gradient-to-r from-[#00A843] to-[#00E65B] shadow-[0_0_15px_rgba(0,200,83,0.8)]" style={{ width: '68%' }}></div>
            <div className="h-full bg-gradient-to-r from-[#FF3B30] to-[#FF5247] shadow-[0_0_15px_rgba(255,59,48,0.8)]" style={{ width: '32%' }}></div>
          </div>
        </div>

        {/* News Feed List */}
        <div className="flex-1 overflow-y-auto p-5 space-y-5 relative z-10 custom-scrollbar">
          {(!newsFeed || !Array.isArray(newsFeed) || newsFeed.length === 0) ? (
            <div className="text-center text-[#888888] font-mono mt-10">AI Analiz ediliyor... Lütfen bekleyin.</div>
          ) : newsFeed.map((news) => {
            const isBull = news.sentiment === 'BULLISH';
            const isBear = news.sentiment === 'BEARISH';
            const glowColor = isBull ? 'rgba(0,200,83,0.15)' : isBear ? 'rgba(255,59,48,0.15)' : 'transparent';
            const borderColor = isBull ? 'border-[#00C853]' : isBear ? 'border-[#FF3B30]' : 'border-[#555555]';
            const badgeBg = isBull ? 'bg-[#00C853]/10 text-[#00C853] border-[#00C853]/30' : isBear ? 'bg-[#FF3B30]/10 text-[#FF3B30] border-[#FF3B30]/30' : 'bg-[#333333]/50 text-[#888888] border-[#555555]/50';
            const Icon = isBull ? TrendingUp : isBear ? TrendingDown : Minus;
            const date = new Date(news.time);
            const timeStr = isNaN(date) ? news.time : date.toLocaleTimeString('tr-TR', { hour: '2-digit', minute: '2-digit' });

            return (
              <div 
                key={news.id} 
                className={`group relative bg-[#0A0A0A]/80 backdrop-blur-md border border-[#1A1A1A] rounded-2xl overflow-hidden transition-all duration-300 border-l-[4px] ${borderColor} hover:-translate-y-1 hover:border-l-[6px]`}
                onMouseEnter={(e) => e.currentTarget.style.boxShadow = `0 10px 30px -10px ${glowColor}, inset 0 0 20px ${glowColor.replace('0.15', '0.05')}`}
                onMouseLeave={(e) => e.currentTarget.style.boxShadow = 'none'}
              >
                <div className="p-5">
                  <div className="flex justify-between items-center mb-4">
                    <div className="text-[11px] text-[#888888] font-mono flex items-center gap-2">
                      <span className="text-white/80 font-bold tracking-wide">{news.source}</span>
                      <div className="w-1 h-1 rounded-full bg-[#555555]"></div>
                      <span>{timeStr}</span>
                    </div>
                    <div className={`px-2.5 py-1 rounded-md text-[10px] font-extrabold tracking-widest flex items-center gap-1.5 border backdrop-blur-sm ${badgeBg}`}>
                      <Icon className="w-3 h-3" />
                      {news.sentiment} | %{Math.round(news.confidence * 100)}
                    </div>
                  </div>
                  <h3 className="text-white font-bold text-[15px] leading-snug mb-3">
                    <NewsContentHighlighter text={news.title} onSymbolClick={setCurrentSymbol} />
                  </h3>
                  <div className="bg-[#111111]/50 rounded-lg p-3 mb-4 border border-[#1A1A1A]">
                    <p className="text-[#A0A0A0] text-xs leading-relaxed font-medium">
                      <span className="text-[#00C853] mr-1.5 font-bold">AI ÖZET:</span>
                      <NewsContentHighlighter text={news.summary} onSymbolClick={setCurrentSymbol} />
                    </p>
                  </div>
                  <div className="flex justify-between items-center mt-2">
                    <div className="flex items-center gap-2">
                      <Flame className={`w-3.5 h-3.5 ${news.impact >= 4 ? (isBull ? 'text-[#00C853]' : 'text-[#FF3B30]') : 'text-[#555555]'}`} />
                      <div className="flex gap-1">
                        {[...Array(5)].map((_, i) => (
                          <div key={i} className={`w-1.5 h-3 rounded-sm transform -skew-x-12 ${i < news.impact ? (isBull ? 'bg-[#00C853] shadow-[0_0_5px_#00C853]' : isBear ? 'bg-[#FF3B30] shadow-[0_0_5px_#FF3B30]' : 'bg-[#888888]') : 'bg-[#222222]'}`}></div>
                        ))}
                      </div>
                    </div>
                    <div className="flex items-center gap-2">
                      <div className="flex gap-1.5">
                        {(news.tags || []).map(tag => (
                          <button 
                            key={tag} 
                            onClick={(e) => { e.stopPropagation(); setCurrentSymbol(`${tag}/USDT`); }}
                            className="text-[10px] font-bold text-[#888888] bg-[#111111] border border-[#222222] hover:bg-[#222222] hover:text-white px-2 py-1 rounded transition-colors"
                          >
                            #{tag}
                          </button>
                        ))}
                      </div>
                      <a href={news.link} target="_blank" rel="noopener noreferrer" className="ml-2 text-[#555555] hover:text-white transition-colors" onClick={(e) => e.stopPropagation()}>
                        <ExternalLink className="w-4 h-4" />
                      </a>
                    </div>
                  </div>
                </div>
              </div>
            );
          })}
        </div>
      </div>

      {/* Main Layout Grid */}
      <div className="px-2 pb-2 pt-3 h-[calc(100vh-61px)]">
        <PanelGroup direction="horizontal" autoSaveId="parabu-pro-layout">
          
        {/* Left Sidebar: Fixed Account Balances & Orders */}
        <Panel defaultSize={20} minSize={15} maxSize={35} className="flex flex-col gap-3 overflow-y-auto custom-scrollbar pb-4 pr-1">
          
          {/* Balance Widget */}
          <div className="bg-[#0A0A0A] border border-[#1A1A1A] rounded-xl p-5 relative overflow-hidden group shrink-0">
            <div className="absolute top-0 right-0 w-32 h-32 bg-[#00C853]/5 rounded-full blur-3xl -mr-10 -mt-10 group-hover:bg-[#00C853]/10 transition-all duration-500"></div>
            
            <div className="flex items-center gap-2 mb-6">
              <Wallet className="w-5 h-5 text-[#00C853]" />
              <h2 className="font-bold text-white text-base">Hesap Bakiyesi</h2>
            </div>
            
            <div className="mb-6">
              <p className="text-[#888888] text-[10px] font-semibold uppercase tracking-wider mb-1">Tahmini Değer (USDT)</p>
              <div className="text-xl md:text-2xl font-bold text-white font-mono flex items-end gap-1 truncate" title={`$${totalBalanceUSDT}`}>
                <span>$</span>{totalBalanceUSDT}
              </div>
            </div>

            <div className="space-y-2">
              <div className="bg-[#111111] p-2.5 rounded-lg border border-[#1A1A1A]">
                <div className="flex justify-between items-center mb-0.5">
                  <span className="text-white text-xs font-semibold flex items-center gap-1.5">
                    <div className="w-1.5 h-1.5 rounded-full bg-[#00C853]"></div> USDT
                  </span>
                  <span className="text-[#00C853] font-mono text-[9px]">Tether</span>
                </div>
                <div className="text-xs font-mono text-[#E0E0E0] truncate">{balances.usdt.toLocaleString(undefined, {minimumFractionDigits: 2, maximumFractionDigits: 2})}</div>
                {balances.usdtLocked > 0 && <div className="text-[9px] text-[#888888] font-mono mt-0.5 truncate">Kilitli: {balances.usdtLocked.toLocaleString()}</div>}
              </div>

              <div className="bg-[#111111] p-2.5 rounded-lg border border-[#1A1A1A]">
                <div className="flex justify-between items-center mb-0.5">
                  <span className="text-white text-xs font-semibold flex items-center gap-1.5">
                    <div className="w-1.5 h-1.5 rounded-full bg-[#F7931A]"></div> BTC
                  </span>
                </div>
                <div className="text-xs font-mono text-[#E0E0E0] truncate">{balances.btc.toFixed(4)}</div>
              </div>
              <div className="bg-[#111111] p-2.5 rounded-lg border border-[#1A1A1A]">
                <div className="flex justify-between items-center mb-0.5">
                  <span className="text-white text-xs font-semibold flex items-center gap-1.5">
                    <div className="w-1.5 h-1.5 rounded-full bg-[#627EEA]"></div> ETH
                  </span>
                </div>
                <div className="text-xs font-mono text-[#E0E0E0] truncate">{balances.eth.toFixed(4)}</div>
              </div>
              <div className="bg-[#111111] p-2.5 rounded-lg border border-[#1A1A1A]">
                <div className="flex justify-between items-center mb-0.5">
                  <span className="text-white text-xs font-semibold flex items-center gap-1.5">
                    <div className="w-1.5 h-1.5 rounded-full bg-[#23292F]"></div> XRP
                  </span>
                </div>
                <div className="text-xs font-mono text-[#E0E0E0] truncate">{balances.xrp.toFixed(2)}</div>
              </div>
            </div>
            
              <button className="w-full mt-5 py-2 bg-[#111111] hover:bg-[#1A1A1A] text-white text-xs font-semibold rounded-lg border border-[#333333] transition-colors flex justify-center items-center gap-2">
                <ArrowDownRight className="w-4 h-4 text-[#00C853]" /> Yatır / Çek
              </button>
            </div>
            
            {/* Active Orders Widget */}
            <div className="bg-[#0A0A0A] border border-[#1A1A1A] rounded-xl flex-1 p-4 flex flex-col min-h-[300px]">
              <h2 className="font-bold text-white mb-4">Açık Emirlerim</h2>
              <div className="flex-1 overflow-hidden pr-2">
                <VirtualOrdersList 
                  orders={activeOrders} 
                  onCancel={cancelOrder} 
                  onUpdatePrice={updateOrderPrice} 
                />
              </div>
            </div>
          </Panel>

          <CustomResizeHandle />

        {/* Center: Chart & Order Form */}
        <Panel defaultSize={55} minSize={40} className="flex flex-col gap-3 min-w-0 pl-1 pr-1">
          
          {/* Chart Panel */}
          <div className="bg-[#0A0A0A] border border-[#1A1A1A] rounded-xl flex-1 flex flex-col overflow-hidden relative">
            <div className="p-4 border-b border-[#1A1A1A] flex justify-between items-center bg-[#050505]">
              <div className="flex items-center gap-4">
                <select 
                  value={currentSymbol} 
                  onChange={(e) => {
                    setCurrentSymbol(e.target.value);
                    setAsks([]);
                    setBids([]);
                    setRecentTrades([]);
                  }}
                  className="bg-[#111111] border border-[#333333] text-2xl font-bold text-white rounded-md px-2 py-1 outline-none"
                >
                  <option value="BTC/USDT">BTC/USDT</option>
                  <option value="ETH/USDT">ETH/USDT</option>
                  <option value="XRP/USDT">XRP/USDT</option>
                </select>
                <span className={`text-2xl font-mono font-bold transition-colors duration-300 ${priceDirection === 'up' ? 'text-[#00C853]' : priceDirection === 'down' ? 'text-[#FF3B30]' : 'text-[#888888]'}`}>
                  {currentPrice.toFixed(2)}
                  {priceDirection === 'up' && <ArrowUpRight className="inline-block w-5 h-5 ml-1" />}
                  {priceDirection === 'down' && <ArrowDownRight className="inline-block w-5 h-5 ml-1" />}
                </span>
              </div>
              <div className="flex items-center gap-2">
                {/* Indicator Toggle Button */}
                <div className="relative">
                  <button 
                    onClick={() => setShowIndicatorMenu(!showIndicatorMenu)}
                    className={`p-2 rounded-lg border transition-colors ${showIndicatorMenu ? 'bg-[#222222] border-[#00C853] text-[#00C853]' : 'bg-[#111111] border-[#1A1A1A] text-[#888888] hover:text-white'}`}
                    title="İndikatörler"
                  >
                    <BarChart3 className="w-4 h-4" />
                  </button>
                  
                  {/* Indicator Dropdown */}
                  {showIndicatorMenu && (
                    <div className="absolute top-full right-0 mt-2 w-56 bg-[#0A0A0A] border border-[#333333] rounded-xl shadow-2xl z-50 p-3 space-y-1">
                      <div className="text-[10px] text-[#888888] font-bold tracking-widest uppercase mb-2 px-2">İNDİKATÖRLER</div>
                      {indicatorConfig.map(ind => (
                        <button
                          key={ind.key}
                          onClick={() => setIndicators(prev => ({ ...prev, [ind.key]: !prev[ind.key] }))}
                          className="flex items-center justify-between w-full px-2 py-1.5 rounded-lg hover:bg-[#111111] transition-colors"
                        >
                          <div className="flex items-center gap-2">
                            <div className="w-3 h-0.5 rounded" style={{ backgroundColor: ind.color }}></div>
                            <span className="text-xs font-semibold text-[#E0E0E0]">{ind.label}</span>
                          </div>
                          {indicators[ind.key] ? (
                            <Eye className="w-3.5 h-3.5 text-[#00C853]" />
                          ) : (
                            <EyeOff className="w-3.5 h-3.5 text-[#555555]" />
                          )}
                        </button>
                      ))}
                    </div>
                  )}
                </div>

                <div className="flex flex-wrap bg-[#111111] rounded-lg p-1 border border-[#1A1A1A]">
                  <button onClick={() => setTimeframe('1m')} className={`px-2 md:px-4 py-1 text-xs font-semibold rounded transition-colors ${timeframe === '1m' ? 'bg-[#222222] text-white shadow' : 'text-[#888888] hover:text-white'}`}>1m</button>
                  <button onClick={() => setTimeframe('15m')} className={`px-2 md:px-4 py-1 text-xs font-semibold rounded transition-colors ${timeframe === '15m' ? 'bg-[#222222] text-white shadow' : 'text-[#888888] hover:text-white'}`}>15m</button>
                  <button onClick={() => setTimeframe('1H')} className={`px-2 md:px-4 py-1 text-xs font-semibold rounded transition-colors ${timeframe === '1H' ? 'bg-[#222222] text-white shadow' : 'text-[#888888] hover:text-white'}`}>1H</button>
                  <button onClick={() => setTimeframe('1D')} className={`px-2 md:px-4 py-1 text-xs font-semibold rounded transition-colors ${timeframe === '1D' ? 'bg-[#222222] text-white shadow' : 'text-[#888888] hover:text-white'}`}>1D</button>
                </div>
              </div>
            </div>

            {/* Main chart area */}
            <div className="flex-1 flex flex-col bg-[#000000] relative min-h-0 overflow-hidden">
              {/* Main price chart */}
              <TradingChart ref={tradingChartRef} currentSymbol={currentSymbol} />
            </div>
          </div>

          {/* Bottom Area: Order Form */}
          <div className="flex gap-4 shrink-0">
            <OrderForm 
              currentSymbol={currentSymbol}
              balances={balances}
              price={price} setPrice={setPrice}
              quantity={quantity} setQuantity={setQuantity}
              orderType={orderType} setOrderType={setOrderType}
              orderSide={orderSide} setOrderSide={setOrderSide}
              onSubmit={submitOrder}
            />
          </div>
        </Panel>

        <CustomResizeHandle />

        {/* Right Panel: Tabs for Order Book, News, Trades */}
        <Panel defaultSize={25} minSize={20} maxSize={40} className="hidden lg:flex shrink-0 pl-1">
          <RightUtilityPanel trades={recentTrades} newsFeed={newsFeed} bids={bids} asks={asks} onSelectPrice={setPrice} />
        </Panel>
        
        </PanelGroup>
      </div>
      <NewsPopover popover={popover} onClose={() => setPopover(null)} />
    </div>
  );
}

export default App;
