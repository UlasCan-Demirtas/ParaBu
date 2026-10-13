import { create } from 'zustand';
import { subscribeWithSelector } from 'zustand/middleware';
import { useShallow } from 'zustand/react/shallow';

// ==========================================
// 1. STATE SLICES
// ==========================================

const createTickerSlice = (set) => ({
  ticker: {
    symbol: 'BTC_USDT',
    lastPrice: 0,
    priceChange: 0,
    priceChangePercent: 0,
    highPrice: 0,
    lowPrice: 0,
    volume: 0,
  },
  updateTicker: (data) => set((state) => ({ ticker: { ...state.ticker, ...data } })),
});

const createOrderBookSlice = (set) => ({
  orderBook: {
    bids: [], // Array of [price, quantity]
    asks: [], // Array of [price, quantity]
    lastUpdateId: 0,
  },
  // Replaces the order book completely (e.g. on initial snapshot load)
  setOrderBook: (bids, asks, updateId) => 
    set(() => ({ orderBook: { bids, asks, lastUpdateId: updateId } })),
});

const createWalletSlice = (set) => ({
  wallet: {
    balances: {},   // { BTC: 1.5, USDT: 50000 }
    locked: {},     // { BTC: 0.1, USDT: 0 }
  },
  updateBalances: (balances, locked) => 
    set((state) => ({ wallet: { ...state.wallet, balances, locked } })),
});

const createAiFeedSlice = (set) => ({
  aiFeed: {
    sentiment: 'NEUTRAL', // BULLISH, BEARISH, NEUTRAL
    sentimentScore: 50,
    marketEvents: [],
  },
  updateAiFeed: (data) => set((state) => ({ aiFeed: { ...state.aiFeed, ...data } })),
});

// ==========================================
// 2. ROOT STORE CREATION
// ==========================================

// We use subscribeWithSelector to allow non-React components (like WebSockets)
// to subscribe to specific state changes efficiently.
export const useTradingStore = create(
  subscribeWithSelector((set, get) => ({
    ...createTickerSlice(set, get),
    ...createOrderBookSlice(set, get),
    ...createWalletSlice(set, get),
    ...createAiFeedSlice(set, get),
  }))
);

// ==========================================
// 3. ATOMIC SELECTORS & SHALLOW COMPARISON
// ==========================================

// BEST PRACTICE: Custom hooks exporting ONLY the specific slices components need.
// By using useShallow, the component will ONLY re-render if the primitive values inside change.

export const useTicker = () => 
  useTradingStore(useShallow((state) => ({
    lastPrice: state.ticker.lastPrice,
    priceChangePercent: state.ticker.priceChangePercent,
    volume: state.ticker.volume,
  })));

export const useOrderBookDepth = () => 
  useTradingStore(useShallow((state) => ({
    bids: state.orderBook.bids,
    asks: state.orderBook.asks,
  })));

export const useWalletBalance = (currency) => 
  useTradingStore((state) => state.wallet.balances[currency] || 0);

// ==========================================
// 4. HIGH-FREQUENCY BATCHING MIDDLEWARE
// ==========================================

// When WebSocket streams send 50 depth updates per second, calling set() directly
// will overwhelm React's render cycle (even with React 18 batching).
// This middleware queues the updates and flushes them exactly at the browser's 
// refresh rate using requestAnimationFrame (60 FPS / ~16.6ms).

let isUpdatePending = false;
let updateBuffer = { ticker: null, orderBook: null, wallet: null, aiFeed: null };

export const dispatchBatchedWSUpdate = (sliceName, payload) => {
  // Accumulate changes in the buffer for the respective slice
  if (updateBuffer[sliceName]) {
    updateBuffer[sliceName] = { ...updateBuffer[sliceName], ...payload };
  } else {
    updateBuffer[sliceName] = payload;
  }

  // Schedule a single React state update on the next animation frame
  if (!isUpdatePending) {
    isUpdatePending = true;
    requestAnimationFrame(() => {
      // Build a unified state update object from the buffer
      const batchedStateUpdate = {};
      
      if (updateBuffer.ticker) batchedStateUpdate.ticker = { ...useTradingStore.getState().ticker, ...updateBuffer.ticker };
      if (updateBuffer.orderBook) batchedStateUpdate.orderBook = { ...useTradingStore.getState().orderBook, ...updateBuffer.orderBook };
      if (updateBuffer.wallet) batchedStateUpdate.wallet = { ...useTradingStore.getState().wallet, ...updateBuffer.wallet };
      if (updateBuffer.aiFeed) batchedStateUpdate.aiFeed = { ...useTradingStore.getState().aiFeed, ...updateBuffer.aiFeed };

      // Apply the atomic update
      useTradingStore.setState(batchedStateUpdate);
      
      // Clear buffer & unlock
      updateBuffer = { ticker: null, orderBook: null, wallet: null, aiFeed: null };
      isUpdatePending = false;
    });
  }
};
