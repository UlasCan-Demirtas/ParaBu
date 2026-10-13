import React, { useMemo } from 'react';
import { motion, AnimatePresence } from 'framer-motion';

export default function OrderForm({ 
  currentSymbol, 
  balances, 
  price, setPrice, 
  quantity, setQuantity, 
  orderType, setOrderType, 
  orderSide, setOrderSide, 
  onSubmit 
}) {
  const [baseCurrency, quoteCurrency] = (currentSymbol || 'BTC/USDT').split('/');

  // Calculate totals
  const total = useMemo(() => {
    const p = parseFloat(price) || 0;
    const q = parseFloat(quantity) || 0;
    return p * q;
  }, [price, quantity]);

  // Handle percentages
  const handlePercent = (side, pct) => {
    if (side === 'BUY') {
      const p = parseFloat(price) || 0;
      if (p <= 0 && orderType === 'LIMIT') return;
      
      const availableQuote = balances[quoteCurrency.toLowerCase()] || 0;
      const budget = availableQuote * pct;
      
      if (orderType === 'LIMIT') {
        setQuantity((budget / p).toFixed(6));
      } else {
        // Market buy - usually quantity is in base currency, but budget is quote.
        // Without an orderbook price oracle, we might just set quantity if backend supports quote qty, 
        // or we need current market price. We will approximate or just leave it.
        // Assuming we need quantity in base currency, we might need a rough price.
        // For simplicity, we just set total if we had a total input, but we have quantity.
        // If it's market, we might disable percentage for BUY if we don't know the price, but let's assume we have `price` as the last ticker price.
        const fallbackPrice = p > 0 ? p : 1; 
        setQuantity((budget / fallbackPrice).toFixed(6));
      }
    } else {
      const availableBase = balances[baseCurrency.toLowerCase()] || 0;
      setQuantity((availableBase * pct).toFixed(6));
    }
  };

  const isBuyError = orderSide === 'BUY' && total > (balances[quoteCurrency.toLowerCase()] || 0);
  const isSellError = orderSide === 'SELL' && (parseFloat(quantity) || 0) > (balances[baseCurrency.toLowerCase()] || 0);

  const handleSubmit = (e, side) => {
    e.preventDefault();
    setOrderSide(side);
    // Let React process the state update, then submit.
    // In actual implementation, we might want to call onSubmit directly with side.
    // Since App.jsx's submitOrder relies on state, we can just call onSubmit(e).
    // Wait, App.jsx uses the state `orderSide`. We should ensure it's set.
    setTimeout(() => onSubmit(e), 0);
  };

  const renderSide = (side) => {
    const isBuy = side === 'BUY';
    const title = isBuy ? 'Alış (Buy)' : 'Satış (Sell)';
    const color = isBuy ? '#00C853' : '#FF3B30';
    const bgHover = isBuy ? 'hover:bg-[#00A843]' : 'hover:bg-[#D63026]';
    const shadowColor = isBuy ? 'rgba(0,200,83,0.39)' : 'rgba(255,59,48,0.39)';
    const btnText = isBuy ? `${baseCurrency} AL` : `${baseCurrency} SAT`;
    const balanceText = isBuy 
      ? `Bakiye: ${(balances[quoteCurrency.toLowerCase()] || 0).toFixed(2)} ${quoteCurrency}`
      : `Bakiye: ${(balances[baseCurrency.toLowerCase()] || 0).toFixed(4)} ${baseCurrency}`;
    
    // Determine active side so we only show errors when user interacts with a specific side
    // Actually, since inputs are shared, both will show error if applicable. We will only highlight the one that has an error.
    const hasError = isBuy ? isBuyError : isSellError;

    return (
      <div className={`flex-1 p-4 rounded-xl bg-[#0A0A0A] border transition-all duration-300 relative overflow-hidden flex flex-col ${
        hasError ? (isBuy ? 'border-[#00C853] shadow-[0_0_15px_rgba(0,200,83,0.25)]' : 'border-[#FF3B30] shadow-[0_0_15px_rgba(255,59,48,0.25)]') : 'border-[#1A1A1A] focus-within:border-[#333333]'
      }`}>
        <div className="flex justify-between items-center mb-4">
          <h4 className="text-white font-bold text-sm tracking-wide">{title}</h4>
          <span className="text-[11px] text-[#888888] font-mono bg-[#111111] px-2 py-1 rounded border border-[#1A1A1A]">{balanceText}</span>
        </div>
        
        {/* Price Input */}
        <div className={`bg-[#050505] rounded-lg flex items-center px-3 py-2.5 mb-3 border transition-colors ${
           orderType === 'MARKET' ? 'border-[#1A1A1A] opacity-60' : 'border-[#222222] focus-within:border-white/20'
        }`}>
          <span className="text-[#888888] text-xs font-semibold w-14">Fiyat</span>
          {orderType === 'MARKET' ? (
            <span className="flex-1 text-right text-white/50 text-xs font-semibold tracking-wide italic">En İyi Piyasa Fiyatı</span>
          ) : (
            <input 
              id={`price-${side}`}
              type="number" step="0.01" 
              value={price} onChange={e => { setPrice(e.target.value); setOrderSide(side); }} 
              className="bg-transparent flex-1 text-right text-white text-sm outline-none font-mono placeholder-[#333333]" 
              placeholder="0.00" 
            />
          )}
          <span className="text-[#888888] font-semibold ml-2 text-xs w-10 text-right">{quoteCurrency}</span>
        </div>
        
        {/* Quantity Input */}
        <div className={`bg-[#050505] rounded-lg flex items-center px-3 py-2.5 mb-3 border transition-colors ${
           hasError ? (isBuy ? 'border-[#00C853]' : 'border-[#FF3B30]') : 'border-[#222222] focus-within:border-white/20'
        }`}>
          <span className="text-[#888888] text-xs font-semibold w-14">Miktar</span>
          <input 
            id={`quantity-${side}`}
            type="number" step="0.0001" 
            value={quantity} onChange={e => { setQuantity(e.target.value); setOrderSide(side); }} 
            className="bg-transparent flex-1 text-right text-white text-sm outline-none font-mono placeholder-[#333333]" 
            placeholder="0.00" 
          />
          <span className="text-[#888888] font-semibold ml-2 text-xs w-10 text-right">{baseCurrency}</span>
        </div>

        {/* Percentage Slider / Buttons */}
        <div className="flex justify-between items-center gap-2 mb-4">
          {[0.25, 0.50, 0.75, 1.0].map(pct => (
            <button
              key={pct}
              type="button"
              onClick={() => { setOrderSide(side); handlePercent(side, pct); }}
              className="flex-1 bg-[#111111] hover:bg-[#222222] border border-[#1A1A1A] hover:border-[#333333] text-[#888888] hover:text-white text-[10px] font-bold py-1.5 rounded transition-colors"
            >
              %{pct * 100}
            </button>
          ))}
        </div>

        {/* Total Calculation */}
        <div className="flex justify-between items-center mb-5 px-1">
          <span className="text-[#888888] text-xs font-semibold">Toplam</span>
          <div className="flex items-center gap-1">
            <span className={`font-mono font-bold text-sm ${hasError ? (isBuy ? 'text-[#00C853]' : 'text-[#FF3B30]') : 'text-white'}`}>
              {orderType === 'MARKET' ? '---' : total.toFixed(2)}
            </span>
            <span className="text-[#888888] text-xs">{quoteCurrency}</span>
          </div>
        </div>

        <div className="mt-auto relative">
          <AnimatePresence>
            {hasError && (
              <motion.div 
                initial={{ opacity: 0, y: 10 }} animate={{ opacity: 1, y: 0 }} exit={{ opacity: 0, y: 10 }}
                className={`absolute -top-8 left-0 right-0 text-center text-[10px] font-bold py-1 rounded ${isBuy ? 'text-[#00C853] bg-[#00C853]/10' : 'text-[#FF3B30] bg-[#FF3B30]/10'}`}
              >
                Yetersiz Bakiye
              </motion.div>
            )}
          </AnimatePresence>
          <button 
            type="submit" 
            onClick={(e) => handleSubmit(e, side)}
            disabled={hasError}
            style={{ backgroundColor: color }}
            className={`w-full text-black font-extrabold text-sm py-3 rounded-lg transition-all shadow-[0_4px_14px_${shadowColor}] hover:shadow-[0_6px_20px_${shadowColor}] transform hover:-translate-y-[1px] disabled:opacity-50 disabled:cursor-not-allowed disabled:transform-none`}
          >
            {btnText}
          </button>
        </div>
      </div>
    );
  };

  return (
    <div className="flex-1 bg-[#050505] border border-[#1A1A1A] rounded-xl p-4 flex flex-col relative">
      {/* Header & Tabs */}
      <div className="flex justify-between items-center mb-4">
        <h3 className="font-bold text-white text-base tracking-wide">Hızlı İşlem</h3>
        <div className="flex bg-[#111111] rounded-lg p-1 border border-[#1A1A1A] relative">
          <button 
            type="button"
            onClick={() => setOrderType('LIMIT')} 
            className={`px-5 py-1.5 text-sm font-semibold rounded-md transition-all relative z-10 ${orderType === 'LIMIT' ? 'text-white' : 'text-[#888888] hover:text-white'}`}
          >
            Limit
          </button>
          <button 
            type="button"
            onClick={() => setOrderType('MARKET')} 
            className={`px-5 py-1.5 text-sm font-semibold rounded-md transition-all relative z-10 ${orderType === 'MARKET' ? 'text-white' : 'text-[#888888] hover:text-white'}`}
          >
            Market
          </button>
          
          {/* Active Tab Background Indicator */}
          <motion.div 
            layoutId="orderTypeBg"
            className="absolute top-1 bottom-1 w-[80px] bg-[#222222] rounded-md shadow z-0"
            animate={{ left: orderType === 'LIMIT' ? '4px' : '78px' }}
            transition={{ type: 'spring', stiffness: 400, damping: 30 }}
          />
        </div>
      </div>

      {/* Forms Container */}
      <form onSubmit={(e) => e.preventDefault()} className="flex gap-4 h-full">
        {renderSide('BUY')}
        {renderSide('SELL')}
      </form>
    </div>
  );
}
