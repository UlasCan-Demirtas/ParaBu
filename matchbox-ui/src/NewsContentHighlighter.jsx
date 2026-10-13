import React from 'react';

const ASSETS = {
  'BITCOIN': 'BTC', 'BTC': 'BTC', '$BTC': 'BTC',
  'ETHEREUM': 'ETH', 'ETH': 'ETH', '$ETH': 'ETH',
  'SOLANA': 'SOL', 'SOL': 'SOL', '$SOL': 'SOL',
  'RIPPLE': 'XRP', 'XRP': 'XRP', '$XRP': 'XRP'
};

const REGS = ['SEC', 'FED', 'FAİZ', 'FAIZ', 'FAİZLER', 'REGULATION', 'POWELL', 'GENSLER', 'REGÜLASYON'];

export const NewsContentHighlighter = ({ text, onSymbolClick, className }) => {
  if (!text || typeof text !== 'string') return null;

  // Regex pattern to capture targeted words and symbols
  const pattern = `(\\bSEC\\b|\\bFED\\b|\\bFaiz\\b|\\bFaizler\\b|\\bRegülasyon\\b|\\bPowell\\b|\\bGensler\\b|\\bBitcoin\\b|\\bEthereum\\b|\\bSolana\\b|\\bRipple\\b|\\bBTC\\b|\\bETH\\b|\\bSOL\\b|\\bXRP\\b|\\$BTC|\\$ETH|\\$SOL|\\$XRP)`;
  
  const regex = new RegExp(pattern, 'gi');
  const parts = text.split(regex);

  return (
    <span className={className}>
      {parts.map((part, i) => {
        if (!part) return null;
        
        const upper = part.toUpperCase();
        
        if (REGS.includes(upper)) {
          return (
            <span key={i} className="bg-[#FF9800]/20 border border-[#FF9800]/30 text-[#FF9800] px-1 py-0.5 rounded font-bold uppercase tracking-wider mx-0.5 text-[0.9em] shadow-[0_0_8px_rgba(255,152,0,0.15)]">
              {part}
            </span>
          );
        }
        
        if (ASSETS[upper]) {
          const symbol = ASSETS[upper];
          return (
            <button
              key={i}
              onClick={(e) => {
                e.stopPropagation(); // prevent modal opening when clicking badge
                if (onSymbolClick) onSymbolClick(`${symbol}/USDT`);
              }}
              className="bg-[#00E5FF]/10 border border-[#00E5FF]/40 text-[#00E5FF] hover:bg-[#00E5FF]/30 px-1 py-0.5 rounded font-bold cursor-pointer transition-colors mx-0.5 text-[0.9em] shadow-[0_0_10px_rgba(0,229,255,0.2)]"
            >
              ${symbol}
            </button>
          );
        }

        return <span key={i}>{part}</span>;
      })}
    </span>
  );
};
