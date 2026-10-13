import React, { useState, useEffect } from 'react';
import { motion, AnimatePresence } from 'framer-motion';
import { X, CheckCircle, AlertOctagon } from 'lucide-react';
import { toastEvent } from './toastSystem';

export default function TradingToastManager() {
  const [toasts, setToasts] = useState([]);

  useEffect(() => {
    const handleNewToast = (toast) => {
      setToasts(prev => [...prev, toast]);
      
      // Auto-dismiss after 4 seconds
      setTimeout(() => {
        setToasts(prev => prev.filter(t => t.id !== toast.id));
      }, 4000);
    };

    toastEvent.add(handleNewToast);
    return () => toastEvent.remove(handleNewToast);
  }, []);

  const removeToast = (id) => {
    setToasts(prev => prev.filter(t => t.id !== id));
  };

  return (
    <div className="fixed bottom-6 right-6 z-[100] flex flex-col gap-3 pointer-events-none">
      <AnimatePresence>
        {toasts.map(toast => (
          <motion.div
            key={toast.id}
            initial={{ opacity: 0, x: 50, scale: 0.95 }}
            animate={{ opacity: 1, x: 0, scale: 1 }}
            exit={{ opacity: 0, x: 20, scale: 0.95 }}
            transition={{ type: 'spring', stiffness: 400, damping: 30 }}
            className={`pointer-events-auto flex items-start gap-3 w-[340px] p-4 bg-[#050505] border rounded-lg shadow-2xl relative overflow-hidden ${
              toast.type === 'success' 
                ? 'border-[#00C853] shadow-[0_4px_24px_rgba(0,200,83,0.15)]' 
                : 'border-[#FF3B30] shadow-[0_4px_24px_rgba(255,59,48,0.15)]'
            }`}
          >
            {/* Background minimal glow */}
            <div className={`absolute inset-0 opacity-10 pointer-events-none ${
              toast.type === 'success' ? 'bg-gradient-to-br from-[#00C853] to-transparent' : 'bg-gradient-to-br from-[#FF3B30] to-transparent'
            }`} />

            {/* Icon */}
            <div className="mt-0.5 relative z-10 shrink-0">
              {toast.type === 'success' ? (
                <CheckCircle className="w-5 h-5 text-[#00C853]" />
              ) : (
                <AlertOctagon className="w-5 h-5 text-[#FF3B30]" />
              )}
            </div>

            {/* Content */}
            <div className="flex-1 flex flex-col relative z-10 font-mono">
              <span className={`text-[11px] font-bold uppercase tracking-wider mb-1 ${
                toast.type === 'success' ? 'text-[#00C853]' : 'text-[#FF3B30]'
              }`}>
                {toast.title || (toast.type === 'success' ? 'Emir Eşleşti' : 'İşlem Reddedildi')}
              </span>
              <span className="text-[#E0E0E0] text-xs leading-snug break-words">
                {toast.msg}
              </span>
            </div>

            {/* Close Button */}
            <button 
              onClick={() => removeToast(toast.id)}
              className="text-[#555555] hover:text-white transition-colors relative z-10 shrink-0 p-1"
            >
              <X className="w-4 h-4" />
            </button>
          </motion.div>
        ))}
      </AnimatePresence>
    </div>
  );
}
