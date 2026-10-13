# ParaBu Matchbox - Enterprise Trading Engine 🚀

**ParaBu Matchbox** is a high-throughput, distributed matching engine and market data execution core designed for institutional cryptocurrency exchanges.

Developed in **Go (Golang)**, it features ACID-compliant PostgreSQL ledger balance control, in-memory matching with Write-Ahead Log (WAL) crash recovery, Transactional Outbox pattern for asynchronous event publishing, and LLM-driven market sentiment analysis.

---

## 🌟 Core Architecture & Features

### 1. In-Memory Matching Engine & Crash Recovery
- **Actor Model Matching:** Asynchronous in-memory channels enabling thread-safe, sub-millisecond execution for Limit and Market orders.
- **Write-Ahead Log (WAL):** High-frequency disk logging paired with periodic snapshots ensures 100% zero data loss and instantaneous RAM recovery upon system reboot.
- **L2 Order Book:** Real-time market depth broadcasted via Gorilla WebSockets to client applications.

### 2. Transactional Outbox & Financial Ledger
- **ACID Double-Entry Accounting:** Row-level locking (`FOR UPDATE`) on PostgreSQL prevents double-spending and balances funds atomically.
- **Transactional Outbox Pattern:** Guarantees dual-write safety between database updates and event streams. A background worker periodically polls the outbox table to broadcast messages.
- **Optimistic Fund Locking:** Balances are locked upon order creation, deducted on match completion, and released immediately upon cancellation.

### 3. AI Sentiment Engine
- **Google Gemini LLM Integration:** Market news headlines and macroeconomic data are parsed via an asynchronous worker pipeline.
- **Sentiment Scoring:** Automated classification into Bullish, Bearish, or Neutral polarities, along with an Impact Score (1-10) streamed via WebSockets (`!news@sentiment`).

### 4. Market Data & TradingView UDF Feed
- **24H Ticker Analytics:** Redis Sorted Sets ($O(1)$ lookup) computing 24h High, Low, Volume, VWAP, and percentage change.
- **UDF Endpoints:** Full specification compliance for TradingView charts (`/config`, `/symbols`, `/history`, `/time`).

### 5. Security & Authentication
- **JWT & Refresh Token Rotation:** Secure session lifecycles using `golang-jwt/v5` backed by Redis token rotation.
- **Sliding-Window Rate Limiting:** IP and User ID throttles safeguarding endpoints against high-concurrency spam.

### 6. Observability
- **Prometheus Metrics:** Exported on `/metrics` (active connections, execution latencies, HTTP throughput).
- **OpenTelemetry Tracing:** Distributed transaction tracking with `otelgin`.
- **WS Ping/Pong:** Real-time network latency probes.

### 7. Geometric Brownian Motion (GBM) Simulator
- **Integrated Market Maker:** Simulates realistic order book liquidity using Geometric Brownian Motion (drift and volatility formulas).
- **HFT Flow:** Emits 10 limit orders per second (every 100ms) to populate live depth in test environments.
- **Automated Teardown:** Test trades are kept on Redis Ring Buffers to keep persistent tables clean and are flushed on exit.

---

## 🛠 Tech Stack

- **Language:** Go 1.23+
- **Database:** PostgreSQL (pgx driver)
- **Cache & Pub/Sub:** Redis (go-redis/v9)
- **Web Framework:** Gin (gin-gonic)
- **Metrics & Tracing:** Prometheus, OpenTelemetry
- **WebSocket:** Gorilla WebSocket
- **Precision Math:** Shopspring Decimal

---

## 🚀 Getting Started

### Prerequisites
- PostgreSQL (port 5432)
- Redis (port 6379)
- Go 1.23+

### Environment Configuration (.env)
```env
DATABASE_URL=postgres://user:password@localhost:5432/parabu_matchbox?sslmode=disable
REDIS_ADDR=localhost:6379
JWT_SECRET=super_secret_key
GEMINI_API_KEY=your_gemini_api_key_here
SANDBOX_MODE=true
SANDBOX_CLEANUP_ON_EXIT=true
```

### Running the Engine
```bash
cd matchbox
go run ./cmd/matchbox
```
Server starts on `http://localhost:8080`.
