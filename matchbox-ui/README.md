# ParaBu Matchbox UI - Institutional Trading Terminal

**ParaBu Matchbox UI** is a high-performance web trading terminal built with **React 19 + Vite**, engineered for real-time cryptocurrency execution, deep market analysis, and high-contrast trader ergonomics.

---

## 🌟 Key Features

### 1. Real-Time Market Data & Order Book
- **WebSocket Streaming:** Native connection subscribing to `depth`, `trade`, `tickers`, and `!news@sentiment` streams.
- **Depth Visualizer Order Book:** Cumulative volume bars for Bids (green `rgba(0, 200, 83, 0.15)`) and Asks (red `rgba(255, 23, 68, 0.15)`), monospace typography (`JetBrains Mono`) to eliminate layout shift, and one-click price targeting callback (`onSelectPrice`).
- **Real-Time Ledger:** Displays free and locked balances for USDT, BTC, ETH, and XRP.

### 2. Advanced Charting & Client-Side Indicators
- **TradingView Lightweight Charts:** Responsive candlestick and area visualization powered by `ResizeObserver`.
- **Client-Side Technical Indicators:**
  - Simple Moving Average (SMA 20)
  - Exponential Moving Averages (EMA 9, 21, 50, 200)
  - Relative Strength Index (RSI 14) sub-panel
  - MACD (12, 26, 9) sub-panel
- **Indicator Controls:** Toggleable menu enabling instant overlay customization.

### 3. AI Sentiment Markers & News Feed
- **`NewsMarkerLayer`:** Live sentiment updates analyzed by Google Gemini LLM pinned onto chart candles (Bullish green arrow, Bearish red arrow, ⚡ Lightning icon for impact >= 4).
- **Interactive News Popover:** Click-to-view modal with headline, timestamp, impact score, and sentiment analysis.
- **AI Sentiment Heatmap & Feed Drawer:** Slide-out utility drawer for macro market sentiment.

### 4. High-Performance Execution & Terminal Ergonomics
- **Institutional Fast Order Form:** Limit & Market order tabs, quick balance percentage selector (25%, 50%, 75%, 100%), and glowing neon-red validation for insufficient funds.
- **Virtualized Trade Feed:** Built with `@tanstack/react-virtual` maintaining a 100-item ring buffer with smooth entry slides and flash row alerts for whale transactions (> 1 BTC).
- **Global Keyboard Shortcuts (`useTradingHotkeys`):**
  - <kbd>B</kbd>: Focus Buy form
  - <kbd>S</kbd>: Focus Sell form
  - <kbd>Esc</kbd>: Blur input fields / close modals
  - <kbd>1</kbd>, <kbd>2</kbd>, <kbd>3</kbd>, <kbd>4</kbd>: Switch chart resolution (1m, 15m, 1h, 1d)
  - <kbd>Space</kbd>: Reset chart scale
- **Modular Dashboard Layout:** Flexible panels with `react-resizable-panels`, neon green hover resizers, and auto-saved layout proportions in `localStorage`.
- **Connection Monitor Badge:** Animated SVG pulse indicator with color-coded latency tracker (< 50ms green, 50-150ms orange, > 150ms red).
- **Terminal Trading Toasts:** Audio-enabled stacked notifications for matched orders, rejections, and execution errors.

---

## 🛠 Tech Stack

- **Framework:** React 19, Vite 8
- **Styling:** Vanilla CSS + TailwindCSS (Modular & Utility classes)
- **Charts:** `lightweight-charts` (TradingView)
- **Virtualization:** `@tanstack/react-virtual`
- **Panels:** `react-resizable-panels`
- **State Management:** Zustand
- **Animations:** Framer Motion
- **Icons:** Lucide React

---

## 🚀 Getting Started

### Installation
```bash
npm install
```

### Development
```bash
npm run dev
```
The application will launch on `http://localhost:5173`, connecting directly to the Go backend on `localhost:8080`.
