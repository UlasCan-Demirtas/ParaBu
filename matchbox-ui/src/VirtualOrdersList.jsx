import React, { useRef, useState } from 'react';
import { useVirtualizer } from '@tanstack/react-virtual';
import { Check, X, Edit2 } from 'lucide-react';

export const VirtualOrdersList = ({ orders, onCancel, onUpdatePrice }) => {
  const parentRef = useRef(null);

  const virtualizer = useVirtualizer({
    count: orders.length,
    getScrollElement: () => parentRef.current,
    estimateSize: () => 140, // Height of card + padding
    overscan: 5,
  });

  return (
    <div 
      ref={parentRef} 
      className="flex-1 overflow-y-auto custom-scrollbar pr-2 h-full"
      style={{ minHeight: '100px' }}
    >
      {orders.length === 0 ? (
        <div className="text-center text-[#888888] text-sm mt-10">Açık emriniz bulunmuyor.</div>
      ) : (
        <div
          style={{
            height: `${virtualizer.getTotalSize()}px`,
            width: '100%',
            position: 'relative',
          }}
        >
          {virtualizer.getVirtualItems().map((virtualItem) => {
            const order = orders[virtualItem.index];
            return (
              <div
                key={virtualItem.key}
                style={{
                  position: 'absolute',
                  top: 0,
                  left: 0,
                  width: '100%',
                  height: `${virtualItem.size}px`,
                  transform: `translateY(${virtualItem.start}px)`,
                  paddingBottom: '12px'
                }}
              >
                <VirtualOrderCard 
                  order={order}
                  onCancel={onCancel}
                  onUpdatePrice={onUpdatePrice}
                />
              </div>
            );
          })}
        </div>
      )}
    </div>
  );
};

const VirtualOrderCard = ({ order, onCancel, onUpdatePrice }) => {
  const [isEditing, setIsEditing] = useState(false);
  const [newPrice, setNewPrice] = useState(order.price || '');

  const isBuy = order.side === 'BUY';
  const colorClass = isBuy ? 'text-[#00C853]' : 'text-[#FF3B30]';
  const bgClass = isBuy ? 'bg-[#00C853]/20' : 'bg-[#FF3B30]/20';
  
  // Fake fill simulation for UI (0% to 100% based on data)
  const filledQty = order.filled || 0; 
  const totalQty = order.qty || 0;
  const fillPercent = totalQty > 0 ? (filledQty / totalQty) * 100 : 0;

  const handleUpdate = () => {
    if (newPrice !== order.price) {
      if (onUpdatePrice) onUpdatePrice(order.id, parseFloat(newPrice), order.symbol);
    }
    setIsEditing(false);
  };

  return (
    <div className="bg-[#111111] border border-[#1A1A1A] p-3 rounded-lg flex flex-col group hover:border-[#333333] transition-colors">
      <div className="flex justify-between items-center mb-3">
        <span className={`text-xs font-bold px-2 py-0.5 rounded ${bgClass} ${colorClass}`}>
          {order.side}
        </span>
        <span className="text-xs text-[#888888]">{order.time}</span>
      </div>
      
      <div className="flex justify-between items-center mb-3">
        <div className="flex flex-col gap-0.5">
          <span className="text-[10px] text-[#555555]">Fiyat (USDT)</span>
          {isEditing ? (
            <div className="flex items-center gap-1">
              <input 
                type="number" 
                value={newPrice} 
                onChange={(e) => setNewPrice(e.target.value)}
                className="bg-[#000000] border border-[#333333] text-white w-20 text-xs px-1.5 py-1 outline-none rounded no-spinners"
                autoFocus
              />
              <button onClick={handleUpdate} className="text-[#00C853] p-1 hover:bg-[#222222] rounded"><Check className="w-3.5 h-3.5" /></button>
              <button onClick={() => setIsEditing(false)} className="text-[#FF3B30] p-1 hover:bg-[#222222] rounded"><X className="w-3.5 h-3.5" /></button>
            </div>
          ) : (
            <div className="flex items-center gap-1 group/edit cursor-pointer" onClick={() => setIsEditing(true)}>
              <span className="text-white font-mono text-sm">{order.price}</span>
              <Edit2 className="w-3 h-3 text-[#555555] group-hover/edit:text-white transition-colors" />
            </div>
          )}
        </div>
        
        <div className="flex flex-col items-end gap-0.5">
          <span className="text-[10px] text-[#555555]">Miktar (BTC)</span>
          <span className="text-white font-mono text-sm">{order.qty}</span>
        </div>
      </div>

      <div className="mb-4 relative group/bar cursor-help" title={`%${fillPercent.toFixed(1)} Doldu`}>
        <div className="w-full h-1 bg-[#1A1A1A] rounded-full overflow-hidden">
          <div 
            className="h-full bg-gradient-to-r from-[#00C853] to-[#00E5FF] transition-all duration-300"
            style={{ width: `${Math.max(fillPercent, 1)}%` }}
          ></div>
        </div>
      </div>

      <button onClick={() => onCancel(order.id, order.symbol)} className="w-full py-1.5 border border-[#FF3B30]/50 text-[#FF3B30] hover:bg-[#FF3B30] hover:text-white rounded text-xs font-semibold transition-colors flex justify-center items-center gap-1">
        İptal Et
      </button>
    </div>
  );
};
