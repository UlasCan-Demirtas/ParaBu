# ParaBu Matchbox - Enterprise Trading Engine 🚀

ParaBu Matchbox, kripto para borsaları için geliştirilmiş yüksek performanslı, dağıtık ve ölçeklenebilir bir eşleştirme motoru (Matching Engine) ve trade altyapısıdır. 

Sistem, Go (Golang) ile geliştirilmiş olup ACID uyumlu PostgreSQL cüzdan yönetimi, in-memory emir eşleştirme, Write-Ahead Log (WAL) ile Crash Recovery, Transactional Outbox Pattern ile asenkron olay yönetimi ve AI destekli duygu analizi içerir.

---

## 🌟 Temel Özellikler (Core Features)

### 1. In-Memory Matching Engine & Crash Recovery
- **Actor Model:** Emirler in-memory kanallar üzerinden asenkron olarak işlenir. Thread-safe ve ultra düşük gecikmeli (low-latency) emir eşleme (Limit, Market).
- **Write-Ahead Log (WAL):** Olası sistem kapanmalarına karşı (Crash) saniyenin kesirlerinde diske yazılan WAL dosyası ve belirli periyotlarla alınan Snapshot'lar sayesinde emir defterleri %100 sıfır veri kaybı ile saniyeler içinde RAM'e geri yüklenir.
- **L2 Order Book:** Derinlik (Depth) verisi WebSocket üzerinden anlık olarak (real-time) istemcilere push edilir.

### 2. Transactional Outbox & Ledger
- **ACID & Row-Level Locking:** PostgreSQL üzerinde `SERIALIZABLE` izolasyon seviyesi ve `FOR UPDATE` satır kilitlemeleri ile "Double Spending" engellenir.
- **Transactional Outbox Pattern:** Veritabanına trade yazılırken "Dual-write" problemini engellemek için olaylar (event) aynı transaction içinde outbox tablosuna yazılır. Ayrı bir Go Worker bu tabloyu okuyarak mesaj kuyruklarına (Redis/NATS vb.) dağıtır.
- **Optimistic Locking:** Emir girildiğinde fonlar önceden kilitlenir (Locked Balance), eşleşme anında tahsil edilir veya iptal anında serbest bırakılır.

### 3. AI Sentiment Engine
- **LLM Entegrasyonu:** Gemini API kullanılarak piyasa haberleri ve duyurular asenkron bir worker havuzu üzerinden okunur. 
- **Duygu Analizi:** Gelen haberler Bullish, Bearish, Neutral olarak sınıflandırılır ve piyasa etkisi (Impact Score 1-10) çıkarılarak WebSocket üzerinden istemcilere akıtılır (AI Feed).

### 4. Piyasa Verileri & TradingView (Data Feed)
- **24H Ticker:** Redis ZSET (Sorted Set) üzerinden son 24 saatin $O(1)$ karmaşıklıkla High, Low, Volume, VWAP ve % Değişim hesaplamaları.
- **UDF Endpoints:** Frontend'deki TradingView Lightweight Charts kütüphanesinin tüketebilmesi için tam uyumlu `/config`, `/symbols`, `/history`, `/time` handler'ları.

### 5. Güvenlik & Kimlik Doğrulama
- **JWT & Refresh Token Rotation:** `golang-jwt/v5` ve Redis kullanılarak güvenli oturum yönetimi. Süresi dolan token'lar Redis tabanlı rotation ile yenilenir.
- **Rate Limiting:** IP ve UserID bazlı, Redis "Sliding Window Log" algoritması ile çalışan Rate Limiter. (Örn: Emir API'leri saniyede max 10 istek, Global 60 istek/dk).

### 6. Gelişmiş Gözlemlenebilirlik (Observability)
- **Prometheus Metrics:** `/metrics` endpoint'i üzerinden aktif websocket bağlantıları, emir eşleme süreleri ve HTTP istekleri metrikleri dışa sunulur.
- **OpenTelemetry:** `otelgin` middleware'i ile dağıtık izleme (Distributed Tracing) desteği.
- **WS Ping/Pong:** İstemci ve sunucu arasındaki ağ gecikmesini (Network Latency - ms) ölçmek için gerçek zamanlı ping-pong mesajlaşması.

### 7. Sandbox: Geometric Brownian Motion Simulator
- **Market Maker Bot:** Sistemde aktif veri olmadığında (test ortamı) devreye giren entegre simülatör.
- **Gerçekçi Veri:** Geometric Brownian Motion (GBM) formülü kullanarak (Drift ve Volatility bazlı) fiyatları hesaplar.
- **High Frequency Trading (HFT) Simülasyonu:** Saniyede 10 defa (her 100ms) gerçek limit emirlerini (Bid/Ask) eşleştirme motoruna ileterek Order Book'u derinleştirir.
- **Graceful Cleanup:** Üretilen test verileri PostgreSQL'i kirletmemek için Redis Ring Buffer üzerinde tutulur ve uygulama kapatılırken otomatik temizlenir.

---

## 💻 Frontend Dashboard (React)

- **Saf Siyah ve Neon Yeşil Tasarım:** ParaBu Proje kurallarına uygun yüksek kontrastlı agresif karanlık mod arayüz.
- **Order Book Heatmap:** Gerçek zamanlı gelen derinlik verilerinin görselleştirilmesi.
- **Teknik İndikatörler (SMA, EMA, RSI):** WebSockets üzerinden gelen son fiyata göre istemci tarafında anlık render edilen indikatörler.
- **Ticker Tape & Flash Effects:** Fiyat değişimlerinde yeşil/kırmızı pırıltı efektleri ve kayan fiyat barları.

---

## 🛠 Teknoloji Yığını (Tech Stack)

- **Dil:** Go 1.23+
- **Veritabanı:** PostgreSQL (pgx driver)
- **Cache & Pub/Sub:** Redis (go-redis/v9)
- **Web Framework:** Gin (gin-gonic)
- **Metrikler & İzleme:** Prometheus (client_golang), OpenTelemetry (otelgin)
- **WebSocket:** Gorilla WebSocket
- **Sayısal Hesaplamalar:** Shopspring Decimal (Hassas finansal matematik)
- **Frontend:** React, Vite, TailwindCSS, TradingView Lightweight Charts, Framer Motion

---

## 🚀 Çalıştırma

### Bağımlılıklar
- PostgreSQL sunucusu (5432)
- Redis sunucusu (6379)
- Node.js (Frontend için)

### Çevresel Değişkenler (.env)
```env
DATABASE_URL=postgres://kullanici:sifre@localhost:5432/parabu_matchbox
REDIS_ADDR=localhost:6379
JWT_SECRET=super_secret_key
GEMINI_API_KEY=your_gemini_api_key_here
```

### Başlatma (Backend)
```bash
cd matchbox
go mod tidy
go run ./cmd/matchbox
```

### Başlatma (Frontend)
```bash
cd matchbox-ui
npm install
npm run dev
```

Sunucu başladığında REST & WS Gateway `:8080` portunda, Prometheus Metrics `:8081` portunda, Frontend arayüzü ise `:3001` portunda yayına girecektir.
