import React, { useState, useEffect } from 'react';
import { motion, AnimatePresence } from 'framer-motion';
import { Zap, Activity, ExternalLink, BrainCircuit, ListOrdered } from 'lucide-react';
import OrderBookTable from './OrderBookTable';
import VirtualizedTradeHistory from './VirtualizedTradeHistory';

const MOCK_TRADES = [
  { id: 1, time: '20:21:45', price: '64,230.50', amount: '0.125', type: 'buy' },
  { id: 2, time: '20:21:44', price: '64,228.00', amount: '1.450', type: 'sell' },
  { id: 3, time: '20:21:42', price: '64,229.00', amount: '0.045', type: 'buy' },
  { id: 4, time: '20:21:41', price: '64,235.10', amount: '2.100', type: 'buy' },
  { id: 5, time: '20:21:39', price: '64,234.00', amount: '0.800', type: 'sell' },
  { id: 6, time: '20:21:38', price: '64,234.00', amount: '0.500', type: 'sell' },
  { id: 7, time: '20:21:35', price: '64,232.00', amount: '0.100', type: 'buy' },
  { id: 8, time: '20:21:34', price: '64,230.00', amount: '0.010', type: 'sell' },
  { id: 9, time: '20:21:33', price: '64,230.50', amount: '3.000', type: 'buy' },
  { id: 10, time: '20:21:30', price: '64,231.00', amount: '1.150', type: 'buy' },
];

const MOCK_AI_NEWS = [
  {
    id: 1,
    title: 'SEC approves new Bitcoin ETF structures',
    impact: 5,
    sentiment: 'bullish',
    time: '2 mins ago',
    parity: 'BTC/USDT',
  },
  {
    id: 2,
    title: 'Whale transfers $500M to cold storage',
    impact: 4,
    sentiment: 'bullish',
    time: '15 mins ago',
    parity: 'BTC/USDT',
  },
  {
    id: 3,
    title: 'Major exchange experiences API downtime',
    impact: 3,
    sentiment: 'bearish',
    time: '45 mins ago',
    parity: 'TOTAL',
  },
  {
    id: 4,
    title: 'Fed signals rate cuts in next quarter',
    impact: 5,
    sentiment: 'bullish',
    time: '1 hour ago',
    parity: 'MACRO',
  },
];

// TradeRow and RecentTrades removed in favor of VirtualizedTradeHistory

const AIFeed = ({ newsFeed }) => {
  const displayNews = newsFeed.length > 0 ? newsFeed : MOCK_AI_NEWS;
  return (
    <div className="flex flex-col h-full overflow-y-auto custom-scrollbar p-3 space-y-4">
      {displayNews.map((news, i) => (
        <motion.div
          key={news.id || i}
          initial={{ opacity: 0, y: 10 }}
          animate={{ opacity: 1, y: 0 }}
          className="bg-[#1a1b23] border border-white/10 rounded-xl p-4 hover:border-indigo-500/50 transition-colors group relative overflow-hidden"
        >
          {/* Subtle glow effect */}
          <div className="absolute inset-0 bg-gradient-to-br from-indigo-500/5 to-purple-500/5 opacity-0 group-hover:opacity-100 transition-opacity" />
          
          <div className="flex justify-between items-start mb-2 relative z-10">
            <span className={`text-[10px] uppercase font-bold tracking-wider px-2 py-1 rounded bg-black/40 border ${
              (news.sentiment || 'neutral').toLowerCase() === 'bullish' ? 'text-green-400 border-green-500/30' : 'text-red-400 border-red-500/30'
            }`}>
              {news.sentiment || 'NEUTRAL'}
            </span>
            <span className="text-xs text-gray-500 flex items-center gap-1">
              <Zap size={12} className={(news.impact || 0) >= 4 ? 'text-yellow-500' : 'text-gray-500'} />
              Impact: {news.impact || 1}/5
            </span>
          </div>
          
          <h4 className="text-sm font-medium text-white mb-3 leading-snug relative z-10">
            {news.title}
          </h4>
          
          <div className="flex justify-between items-center relative z-10">
            <span className="text-xs text-gray-500">{news.time || new Date().toLocaleTimeString()}</span>
            <button className="flex items-center gap-1 text-xs font-semibold text-indigo-400 hover:text-indigo-300 bg-indigo-500/10 hover:bg-indigo-500/20 px-2 py-1 rounded transition-colors">
              <ExternalLink size={12} />
              {news.parity || news.symbol || 'MARKET'}
            </button>
          </div>
        </motion.div>
      ))}
    </div>
  );
};

export default function RightUtilityPanel({ trades = [], newsFeed = [], bids = [], asks = [], onSelectPrice }) {
  const [activeTab, setActiveTab] = useState('orderbook'); // Default to Order Book
  const [unreadNews, setUnreadNews] = useState(1);

  const tabs = [
    { id: 'orderbook', label: 'Order Book', icon: <ListOrdered size={14} /> },
    { id: 'ai', label: 'AI Sentiment', icon: <BrainCircuit size={14} /> },
    { id: 'trades', label: 'Recent Trades', icon: <Activity size={14} /> },
  ];

  return (
    <div className="flex flex-col bg-[#0b0c10] border-l border-white/10 h-full w-[320px] shrink-0 font-inter">
      {/* Header Tabs */}
      <div className="flex bg-[#12131a] p-1 border-b border-white/5">
        {tabs.map((tab) => {
          const isActive = activeTab === tab.id;
          return (
            <button
              key={tab.id}
              onClick={() => {
                setActiveTab(tab.id);
                if (tab.id === 'ai') setUnreadNews(0);
              }}
              className={`relative flex-1 flex items-center justify-center gap-2 py-3 text-xs font-semibold rounded-t-lg transition-colors ${
                isActive ? 'text-white' : 'text-gray-500 hover:text-gray-300'
              }`}
            >
              {tab.icon}
              {tab.label}
              
              {/* Notification Badge */}
              {tab.id === 'ai' && unreadNews > 0 && (
                <span className="absolute top-2 right-4 flex h-4 w-4 items-center justify-center rounded-full bg-indigo-500 text-[9px] font-bold text-white">
                  {unreadNews}
                </span>
              )}

              {/* Active Tab Indicator (Framer Motion) */}
              {isActive && (
                <motion.div
                  layoutId="activeTabIndicator"
                  className="absolute bottom-0 left-0 right-0 h-0.5 bg-indigo-500"
                  initial={false}
                  transition={{ type: 'spring', stiffness: 500, damping: 30 }}
                />
              )}
            </button>
          );
        })}
      </div>

      {/* Tab Content Area */}
      <div className="flex-1 relative overflow-hidden bg-[#0b0c10]">
        <AnimatePresence mode="wait">
          <motion.div
            key={activeTab}
            initial={{ opacity: 0, x: activeTab === 'ai' ? -20 : 20 }}
            animate={{ opacity: 1, x: 0 }}
            exit={{ opacity: 0, x: activeTab === 'ai' ? 20 : -20 }}
            transition={{ duration: 0.2, ease: 'easeInOut' }}
            className="absolute inset-0"
          >
            {activeTab === 'trades' ? <VirtualizedTradeHistory trades={trades.length > 0 ? trades : MOCK_TRADES} /> : activeTab === 'ai' ? <AIFeed newsFeed={newsFeed} /> : <OrderBookTable bids={bids} asks={asks} onSelectPrice={onSelectPrice} />}
          </motion.div>
        </AnimatePresence>
      </div>
      
      <style dangerouslySetInnerHTML={{__html: `
        .custom-scrollbar::-webkit-scrollbar {
          width: 4px;
        }
        .custom-scrollbar::-webkit-scrollbar-track {
          background: transparent;
        }
        .custom-scrollbar::-webkit-scrollbar-thumb {
          background-color: rgba(255, 255, 255, 0.1);
          border-radius: 10px;
        }
        .custom-scrollbar:hover::-webkit-scrollbar-thumb {
          background-color: rgba(255, 255, 255, 0.2);
        }
      `}} />
    </div>
  );
}
