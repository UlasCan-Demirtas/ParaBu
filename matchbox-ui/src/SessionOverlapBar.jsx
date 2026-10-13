import React, { useState, useEffect } from 'react';
import { Globe, AlertTriangle } from 'lucide-react';

const SESSIONS = [
  { name: 'Sydney', start: 22, end: 7 },
  { name: 'Tokyo', start: 0, end: 9 },
  { name: 'London', start: 8, end: 17 },
  { name: 'New York', start: 13, end: 22 },
];

export const SessionOverlapBar = () => {
  const [time, setTime] = useState(new Date());

  useEffect(() => {
    const timer = setInterval(() => setTime(new Date()), 1000);
    return () => clearInterval(timer);
  }, []);

  const currentHourUTC = time.getUTCHours();
  
  // Check if session is open
  const isSessionOpen = (session) => {
    if (session.start > session.end) {
      // Overnight (e.g. Sydney 22:00 to 07:00)
      return currentHourUTC >= session.start || currentHourUTC < session.end;
    }
    return currentHourUTC >= session.start && currentHourUTC < session.end;
  };

  const activeSessions = SESSIONS.filter(isSessionOpen);
  const isVolatile = activeSessions.some(s => s.name === 'London') && activeSessions.some(s => s.name === 'New York');

  return (
    <div className="flex items-center gap-3 text-xs font-mono ml-4">
      <div className="flex items-center gap-1.5 px-2 py-1 bg-[#111111] border border-[#1A1A1A] rounded text-[#888888] shadow-inner">
        <Globe className="w-3.5 h-3.5 text-blue-500" />
        <span>UTC {time.toISOString().substring(11, 19)}</span>
      </div>
      
      <div className="flex gap-2 items-center">
        {SESSIONS.map(s => {
          const isOpen = isSessionOpen(s);
          return (
            <div 
              key={s.name} 
              className={`px-2 py-0.5 rounded border transition-colors flex items-center gap-1.5 ${isOpen ? 'bg-[#00C853]/10 border-[#00C853]/30 text-[#00C853]' : 'bg-[#111111] border-[#1A1A1A] text-[#555555]'}`}
            >
              <div className={`w-1.5 h-1.5 rounded-full ${isOpen ? 'bg-[#00C853] shadow-[0_0_5px_#00C853]' : 'bg-[#333333]'}`}></div>
              {s.name}
            </div>
          );
        })}
      </div>

      {isVolatile && (
        <div className="flex items-center gap-2 px-3 py-0.5 bg-[#00E5FF]/10 border border-[#00E5FF]/30 rounded text-[#00E5FF] shadow-[0_0_15px_rgba(0,229,255,0.15)] animate-pulse">
          <AlertTriangle className="w-3.5 h-3.5" />
          <span className="font-bold tracking-wide">YÜKSEK VOLATİLİTE PENCERESİ (London/NY Overlap)</span>
        </div>
      )}
    </div>
  );
};
