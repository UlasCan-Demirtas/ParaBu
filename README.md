# ParaBu ⚡

**ParaBu**, yüksek performanslı, dağıtık ve kurumsal standartlarda geliştirilmiş yeni nesil kripto varlık alım/satım (trading) ve eşleştirme ekosistemidir.

---

## 🏗 Mimari Bileşenler

Proje 3 temel modülden oluşmaktadır:

```
ParaBu/
├── matchbox/       # 🚀 Go (Golang) tabanlı Yüksek Performanslı Matching Engine & Core API
├── matchbox-ui/    # 💻 React + Vite tabanlı Profesyonel Web Trading Terminali
└── hookstorm/      # ⚡ Güvenli & Ölçeklenebilir Webhook Dağıtım Servisi (HMAC SHA-256 & DLQ)
```

### 1. `matchbox` (Matching Engine & Backend API)
- **Actor Model Matching Engine:** In-memory, iş parçacığı güvenli (thread-safe), mikrosaniye seviyesinde Limit ve Market emir eşleştirme.
- **Write-Ahead Log (WAL) & Crash Recovery:** Sıfır veri kaybı hedefiyle periyodik snapshot ve WAL tabanlı anında RAM restorasyonu.
- **Gerçek Zamanlı Veri Akışı:** Gorilla WebSocket üzerinden L2 Order Book (Depth), Trade akışı, Ticker ve AI Duygu analiz beslemesi.
- **ACID Uyumlu Ledger & Outbox:** PostgreSQL üzerinde row-level locking ile çifte harcama (double spending) koruması ve Transactional Outbox pattern.
- **AI Destekli Sentiment Analizi:** Piyasa haberlerini asenkron LLM havuzu üzerinden puanlayan ve etki skoru üreten yapay zekâ entegrasyonu.
- **GBM HFT Simülatörü:** Geometric Brownian Motion tabanlı gerçekçi piyasa yapıcı (Market Maker) botu.

### 2. `matchbox-ui` (Trading Terminal & Dashboard)
- **Saf Siyah & Neon Yeşil Tema:** Profesyonel trader'lar için yüksek kontrastlı modern karanlık arayüz (`#050505` & `#00C853`).
- **Canlı Order Book & Depth Visualizer:** Alış/satış derinlik çubukları, layout-shift korumalı monospace rakamlar ve tek tıkla fiyat seçimi.
- **TradingView Grafikleri:** Lightweight Charts ile dinamik boyutlandırma, SMA, EMA, RSI, MACD göstergeleri.
- **Hızlı İşlem Paneli:** Limit & Market sekmeleri, bakiye yüzdesi butonları (%25, %50, %75, %100) ve istemci doğrulaması.
- **Sanal Listeli İşlem Geçmişi (Virtualized Trade Feed):** `@tanstack/react-virtual` ile bellek dostu, yüksek frekanslı kaydırma ve büyük işlem pırıltı efektleri.
- **Global Kısayollar (Trading Hotkeys):** B (Alış), S (Satış), ESC (Kapat), 1-4 (Zaman aralığı), Space (Ölçek sıfırlama).
- **Esnek Panel Düzeni:** `react-resizable-panels` ile ayarlanabilir ve `localStorage` üzerinde saklanan modüler düzen.

### 3. `hookstorm` (Webhook Dispatcher)
- **Kriptografik İmzalama:** ParaBu standartlarında HMAC SHA-256 imzası (`X-ParaBu-Signature`).
- **Hız Sınırlama (Rate Limiting):** Redis tabanlı sliding window ile uç nokta koruması.
- **Dead Letter Queue (DLQ):** İletilemeyen olaylar için PostgreSQL destekli hata kuyruğu ve yeniden deneme (retry) mekanizması.

---

## 🚀 Hızlı Başlangıç

### Gereksinimler
- **Go:** 1.23+
- **Node.js:** 18+ & npm
- **PostgreSQL:** 14+
- **Redis:** 6+

### 1. Backend'i Başlatma (`matchbox`)
```bash
cd matchbox
cp .env.example .env
# .env dosyasını yapılandırın
go run ./cmd/matchbox
```

### 2. Frontend'i Başlatma (`matchbox-ui`)
```bash
cd matchbox-ui
npm install
npm run dev
```
Tarayıcınızda `http://localhost:5173` adresini açarak terminale erişebilirsiniz.

### 3. Webhook Dağıtıcısını Başlatma (`hookstorm`)
```bash
cd hookstorm
go run ./cmd/hookstorm
```

---

## 🔒 Lisans & Güvenlik
Bu proje gizli ve ticari kullanım standartlarına uygun olarak geliştirilmiştir.
