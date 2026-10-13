import React from 'react';

export default function ConnectionStatusBadge({ status, ping }) {
  const isConnected = status === 'OPEN';
  
  // Dynamic color for latency
  let pingColor = 'text-[#00C853]'; // < 50ms (Green)
  if (ping >= 50 && ping <= 150) {
    pingColor = 'text-orange-400'; // 50-150ms (Orange)
  } else if (ping > 150) {
    pingColor = 'text-[#FF3B30]'; // > 150ms (Red)
  }

  return (
    <div className="flex items-center gap-3 bg-[#111111] border border-[#1A1A1A] px-3 py-1.5 rounded-full">
      {/* SVG Pulsing Dot */}
      <div className="relative flex items-center justify-center w-3 h-3">
        {isConnected ? (
          <>
            <div className="absolute inset-0 bg-[#00C853] rounded-full animate-ping opacity-75"></div>
            <div className="relative w-2 h-2 bg-[#00C853] rounded-full shadow-[0_0_8px_#00C853]"></div>
          </>
        ) : (
          <>
            <div className="absolute inset-0 bg-[#FF3B30] rounded-full animate-ping opacity-75"></div>
            <div className="relative w-2 h-2 bg-[#FF3B30] rounded-full shadow-[0_0_8px_#FF3B30]"></div>
          </>
        )}
      </div>
      
      {/* Text Info */}
      <div className="flex flex-col justify-center">
        <span className={`font-bold text-[10px] tracking-wider uppercase leading-none ${isConnected ? 'text-white' : 'text-[#FF3B30]'}`}>
          {isConnected ? 'ENGINE LIVE' : 'Yeniden Bağlanıyor...'}
        </span>
        {isConnected && (
          <span className={`font-mono text-[10px] font-bold leading-none mt-1 ${pingColor}`}>
            Lat: {ping}ms
          </span>
        )}
      </div>
    </div>
  );
}
