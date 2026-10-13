# ParaBu Matchbox UI - Trading Dashboard

ParaBu Matchbox projesinin **React + Vite** altyapısıyla geliştirilmiş, gerçek zamanlı alım/satım (trading) ve analiz arayüzüdür. Saf Siyah (`#050505`) ve Neon Yeşil (`#00C853`) renk paleti kullanılarak, profesyonel trader'ların ihtiyaçları doğrultusunda odaklı ve minimalist bir tasarım benimsenmiştir.

---

## 🌟 Mevcut Özellikler (Neler Yaptık?)

### 1. Gerçek Zamanlı Veri Akışı ve Orderbook
- **WebSocket Hub:** Backend üzerinden `depth`, `trade`, `tickers` ve `!news@sentiment` topic'lerine abone olunarak anlık veri akışı sağlanır.
- **Canlı Order Book (Emir Defteri):** Bids ve Asks (Alış/Satış) listesi anlık render edilir. Fiyat yığılmalarını (Heatmap mantığı) göstermek için hacme dayalı arka plan yoğunluk animasyonu uygulanmıştır.
- **Canlı Bakiye (Ledger):** Kullanıcının USDT, BTC, ETH ve XRP bakiyeleri (boşta ve kilitli tutarlar) sol panelden anlık izlenebilir.

### 2. Gelişmiş Grafikler ve İndikatörler
- **TradingView Lightweight Charts:** Yüksek performanslı fiyat grafiği (Area/Candlestick) entegre edilmiştir. `ResizeObserver` kullanılarak grafiğin boyutlandırma (kaybolma/kesilme) sorunları çözülmüştür.
- **Teknik İndikatörler (Client-Side):** Tarayıcı üzerinde hesaplanıp anlık olarak grafiğe bindirilen indikatörler:
  - SMA (20), EMA (9, 21, 50, 200)
  - RSI (14) Alt paneli
  - MACD (12,26,9) Alt paneli
- **İndikatör Yönetimi:** UI üzerindeki özel "İndikatörler" menüsünden her bir indikatör ayrı ayrı açılıp kapatılabilir.

### 3. Yapay Zeka (AI) Haber Entegrasyonu ve Sentiment Analizi
- **NewsMarkerLayer:** Gemini tarafından analiz edilen güncel piyasa haberleri (Sentiment) canlı olarak WebSocket üzerinden dinlenir ve **doğrudan fiyat grafiği üzerinde marker (ok ve şimşek ikonları)** olarak işaretlenir.
  - Yeşil Ok: Bullish (Olumlu) haber
  - Kırmızı Ok: Bearish (Olumsuz) haber
  - ⚡ Şimşek: Yüksek Etkili (High Impact >= 4) haber
- **Mikro Modal (Popover):** Grafikteki haber ikonlarına tıklandığında absolü (absolute) olarak pozisyonlanan bir popover üzerinden haber başlığı, zamanı ve etkisi detaylı bir şekilde gösterilir.
- **AI Feed Drawer:** Sağ panelden açılabilen "AI Feed" çekmecesi (drawer) sayesinde piyasadaki genel sentiment izlenebilir.

### 4. Etkileşim ve Ticker Tape
- **Ticker Tape (Kayan Yazı):** Üst barda tüm paritelerin fiyat değişimlerini gösteren sürekli kayan (marquee) bant.
- **Sesli Uyarı (Web Audio API):** Gerçek zamanlı bir trade (eşleşme) gerçekleştiğinde sesli geri bildirim çalar (kapatılabilir).

---

## 🛠 Teknoloji Yığını (Tech Stack)

- **Framework:** React 18, Vite
- **Stil:** Vanilla CSS (Modüler) + Tailwind (Utility class'lar için hafif entegrasyon)
- **Grafik Motoru:** `lightweight-charts` (TradingView)
- **WebSocket:** Native Browser API (`WebSocket`)
- **İkonlar:** Lucide React

---

## 🚀 Çalıştırma ve Kurulum

### Kurulum
```bash
npm install
```

### Geliştirme (Development) Ortamı
```bash
npm run dev
```
Uygulama varsayılan olarak `http://localhost:5173` portunda ayağa kalkacaktır. Backend API'sine (`localhost:8080`) CORS ve WebSocket (`ws://localhost:8080/ws`) üzerinden doğrudan bağlanacak şekilde konfigüre edilmiştir.
