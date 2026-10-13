import { useEffect } from 'react';

export function useTradingHotkeys({ 
  onBuyFocus, 
  onSellFocus, 
  onEscape, 
  onTimeframeChange, 
  onResetChart 
}) {
  useEffect(() => {
    const handleKeyDown = (e) => {
      // Ignore if user is already typing in an input or textarea
      const target = e.target;
      if (
        target.tagName === 'INPUT' || 
        target.tagName === 'TEXTAREA' || 
        target.isContentEditable
      ) {
        // Allow ESC to blur inputs
        if (e.key === 'Escape') {
          target.blur();
          if (onEscape) onEscape();
        }
        return;
      }

      switch (e.key.toLowerCase()) {
        case 'b':
          e.preventDefault();
          if (onBuyFocus) onBuyFocus();
          break;
        case 's':
          e.preventDefault();
          if (onSellFocus) onSellFocus();
          break;
        case 'escape':
          e.preventDefault();
          if (onEscape) onEscape();
          break;
        case '1':
          e.preventDefault();
          if (onTimeframeChange) onTimeframeChange('1m');
          break;
        case '2':
          e.preventDefault();
          if (onTimeframeChange) onTimeframeChange('15m');
          break;
        case '3':
          e.preventDefault();
          if (onTimeframeChange) onTimeframeChange('1h');
          break;
        case '4':
          e.preventDefault();
          if (onTimeframeChange) onTimeframeChange('1d');
          break;
        case ' ': // Space key
          e.preventDefault();
          if (onResetChart) onResetChart();
          break;
        default:
          break;
      }
    };

    window.addEventListener('keydown', handleKeyDown);
    return () => window.removeEventListener('keydown', handleKeyDown);
  }, [onBuyFocus, onSellFocus, onEscape, onTimeframeChange, onResetChart]);
}
