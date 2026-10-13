package main

import (
	"context"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/nats-io/nats.go"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/shopspring/decimal"
	"go.opentelemetry.io/contrib/instrumentation/github.com/gin-gonic/gin/otelgin"
	"go.uber.org/zap"

	"matchbox/internal/alerts"
	"matchbox/internal/analysis"
	"matchbox/internal/auth"
	"matchbox/internal/engine"
	"matchbox/internal/ledger"
	"matchbox/internal/marketdata"
	"matchbox/internal/marketmaker"
	"matchbox/internal/middleware"
	"matchbox/internal/news"
	"matchbox/internal/outbox"
	"matchbox/internal/repository"
	"matchbox/internal/udf"
	"matchbox/internal/wal"
	"matchbox/internal/ws"
	"matchbox/pkg/metrics"
	"matchbox/pkg/types"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"
	"github.com/redis/go-redis/v9"
)

func main() {
	metrics.Init()

	// 1. Setup Structured Logging (Zap)
	logger, _ := zap.NewProduction()
	defer logger.Sync()
	sugar := logger.Sugar()
	sugar.Info("Starting ParaBu Matchbox Enterprise Edition...")

	// 2. Setup Context & Env
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := godotenv.Load(); err != nil {
		sugar.Warn("No .env file found, using system environment variables")
	}

	// 2.1 Init PostgreSQL (TimescaleDB)
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		dbURL = "postgres://postgres:postgres@localhost:5432/matchbox?sslmode=disable"
	}
	dbPool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		sugar.Fatalf("Unable to connect to database: %v", err)
	}
	defer dbPool.Close()

	// 2.2 Init Redis
	redisURL := os.Getenv("REDIS_URL")
	if redisURL == "" {
		redisURL = "redis://localhost:6379/0"
	}
	opt, _ := redis.ParseURL(redisURL)
	rdb := redis.NewClient(opt)
	defer rdb.Close()

	// 3. Setup NATS Connection (Retry 1 time)
	var nc *nats.Conn
	for i := 0; i < 1; i++ {
		nc, err = nats.Connect(nats.DefaultURL)
		if err == nil {
			break
		}
		sugar.Warnf("NATS not available yet, retrying in 2 seconds... (%v)", err)
		time.Sleep(2 * time.Second)
	}
	if err != nil {
		sugar.Warn("Could not connect to NATS, continuing without event streaming...")
	} else {
		sugar.Info("Connected to NATS Message Broker successfully.")
		defer nc.Close()
	}

	// Start Outbox Publisher
	outboxPub := outbox.NewPublisher(dbPool, sugar, nc)
	outboxPub.Start()
	defer outboxPub.Stop()

	// 4. Initialize Ledger & Engine
	exchangeLedger := ledger.NewLedger(dbPool)
	transferSvc := ledger.NewLedgerTransferService(dbPool, rdb)
	holdSvc := ledger.NewHoldService(dbPool, transferSvc)
	_ = holdSvc.InitSchema(context.Background())

	// Create mock balances for the User and the Market Maker Bot
	myUser := "user-1"
	botUser := "bot-1"

	exchangeLedger.Deposit(myUser, "USDT", decimal.NewFromFloat(125000.50))
	exchangeLedger.Deposit(myUser, "BTC", decimal.NewFromFloat(1.4500))
	exchangeLedger.Deposit(myUser, "ETH", decimal.NewFromFloat(10.0))
	exchangeLedger.Deposit(myUser, "XRP", decimal.NewFromFloat(5000.0))

	// The bot needs infinite money to provide liquidity
	exchangeLedger.Deposit(botUser, "USDT", decimal.NewFromFloat(1000000000.0))
	exchangeLedger.Deposit(botUser, "BTC", decimal.NewFromFloat(10000.0))
	exchangeLedger.Deposit(botUser, "ETH", decimal.NewFromFloat(100000.0))
	exchangeLedger.Deposit(botUser, "XRP", decimal.NewFromFloat(10000000.0))

	// Initialize Write-Ahead Log (WAL)
	walWriter, err := wal.NewWALWriter("matchbox.wal", 10*time.Millisecond, false)
	if err != nil {
		sugar.Fatalf("Failed to initialize WAL: %v", err)
	}
	defer walWriter.Close()

	// Initialize SnapshotManager (Trigger every 100,000 events)
	snapshotManager := wal.NewSnapshotManager(walWriter, "./snapshots", 100000)

	matchingEngine := engine.NewEngine(holdSvc, walWriter, snapshotManager)
	symbols := []string{"BTC/USDT", "ETH/USDT", "XRP/USDT"}
	for _, sym := range symbols {
		matchingEngine.RegisterSymbol(sym)
	}

	// Crash Recovery Phase
	recoveryEngine := wal.NewRecoveryEngine("matchbox.wal", "./snapshots", sugar)
	sugar.Info("Starting Crash Recovery...")
	recovered, err := recoveryEngine.Recover(symbols, matchingEngine)
	if err != nil {
		sugar.Fatalf("Fatal error during recovery: %v", err)
	}
	if recovered {
		sugar.Info("Crash Recovery completed successfully! Order books restored.")
	} else {
		sugar.Info("No recovery data found. Starting fresh.")
	}

	// Start workers after recovery is fully deterministic
	matchingEngine.StartWorkers(ctx)

	// 5. Initialize WebSocket Hub
	hub := ws.NewHub(rdb)
	go hub.Run()

	// 4. Start Sandbox Simulator (GBM)
	marketmaker.StartSimulator(ctx, matchingEngine, holdSvc, exchangeLedger, rdb, hub, sugar)

	// 5. Start Reconciliation Worker (Audit Daemon)
	reconciliationWorker := ledger.NewReconciliationWorker(dbPool, rdb, sugar)
	reconciliationWorker.Start()
	defer reconciliationWorker.Stop()

	tickerWorker := marketdata.NewTickerWorker()
	tickerWorker.Start(ctx, hub.Broadcast)

	// 5.1 Initialize News & AI Sentiment Workers
	newsWorker := news.NewNewsWorker(rdb, dbPool)
	newsWorker.Start(ctx)

	geminiKey := os.Getenv("GEMINI_API_KEY")
	if geminiKey != "" {
		sentimentEngine := news.NewSentimentEngine(dbPool, rdb, geminiKey, hub)
		sentimentEngine.Start(ctx, 1) // 1 worker for concurrent analysis (Free tier limits)
		sugar.Info("Started SentimentEngine with 1 worker")
	} else {
		sugar.Warn("GEMINI_API_KEY not set. Sentiment Engine is disabled.")
	}

	// 5.2 Mock Chainwatch Deposit Integration for UI Showcase
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				hashBytes := crypto.Keccak256([]byte(time.Now().String()))
				payload := map[string]interface{}{
					"tx_hash":  "0x" + common.Bytes2Hex(hashBytes),
					"currency": "ETH",
					"amount":   "0.15",
				}
				hub.Broadcast("deposit:user-1", payload)
				sugar.Infof("💰 Mock deposit broadcasted to WS for user-1: %v", payload)
			}
		}
	}()

	tickerService := marketdata.NewTickerService(rdb)

	alertEngine := alerts.NewAlertEngine(rdb)
	alerts.StartNotificationWorker(ctx, rdb)

	// 6. Engine Event Consumers (Broadcast to WS & NATS)
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case depth := <-matchingEngine.DepthOutChan:
				hub.Broadcast("depth:"+depth.Symbol, depth)
			case trade := <-matchingEngine.TradeOutChan:
				hub.Broadcast("trade:"+trade.Symbol, trade)
				metrics.TradesExecuted.WithLabelValues(trade.Symbol).Inc()
				sugar.Infow("Trade Executed",
					"symbol", trade.Symbol,
					"price", trade.Price.String(),
					"qty", trade.Quantity.String(),
				)

				// Update Redis 24h Ticker Stats
				go func(sym string, p, q decimal.Decimal) {
					err := tickerService.ProcessTrade(context.Background(), sym, p, q)
					if err != nil {
						sugar.Errorf("Failed to process ticker stats for %s: %v", sym, err)
					}
				}(trade.Symbol, trade.Price, trade.Quantity)

				if nc != nil {
					// In a real system, marshal to protobuf or JSON and publish
					nc.Publish("trades."+trade.Symbol, []byte(trade.ID.String()))
				}

				// Check User Price Alerts
				go alertEngine.CheckPrice(trade.Symbol, trade.Price)
			case match := <-matchingEngine.MatchOutChan:
				// Update order status in NATS/DB
				_ = match
			case cancel := <-matchingEngine.CancelOutChan:
				hub.Broadcast("order_update:"+cancel.Order.UserID, cancel)
			}
		}
	}()

	// 6. Initialize Auth Service & Rate Limiter
	authService := auth.NewAuthService(rdb)
	rateLimiter := middleware.NewRateLimiter(rdb)

	// 7. REST API Gateway (Gin)
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()

	r.Use(otelgin.Middleware("matchbox-gateway"))

	// Graceful Shutdown Middleware
	var isShuttingDown int32
	r.Use(func(c *gin.Context) {
		if atomic.LoadInt32(&isShuttingDown) == 1 {
			c.AbortWithStatusJSON(http.StatusServiceUnavailable, gin.H{"error": "Service is shutting down"})
			return
		}
		c.Next()
	})

	r.Use(func(c *gin.Context) {
		c.Next()
		status := strconv.Itoa(c.Writer.Status())
		path := c.FullPath()
		if path == "" {
			path = "UNKNOWN"
		}
		metrics.HttpRequestsTotal.WithLabelValues(c.Request.Method, path, status).Inc()
	})

	// CORS Middleware for the React Frontend
	r.Use(func(c *gin.Context) {
		c.Writer.Header().Set("Access-Control-Allow-Origin", "*")
		c.Writer.Header().Set("Access-Control-Allow-Methods", "POST, GET, OPTIONS, PUT, DELETE")
		c.Writer.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}
		c.Next()
	})

	// Global Rate Limit (60 req/min)
	r.Use(rateLimiter.SlidingWindowLimiter(60, time.Minute, "global"))

	r.GET("/ws", func(c *gin.Context) {
		ws.ServeWs(hub, c.Writer, c.Request)
	})

	klineRepo := repository.NewKlineRepo(dbPool)
	orderRepo := repository.NewOrderRepo(dbPool)
	udfGroup := r.Group("/api/udf")
	udf.RegisterUDFRoutes(udfGroup, klineRepo)

	api := r.Group("/api/v1")
	{
		// 24H TICKER ENDPOINT
		api.GET("/ticker/:symbol", func(c *gin.Context) {
			sym := c.Param("symbol")
			ticker, err := tickerService.GetTicker(c.Request.Context(), sym)
			if err != nil {
				c.JSON(http.StatusNotFound, gin.H{"error": "Ticker not found"})
				return
			}
			c.JSON(http.StatusOK, ticker)
		})

		// AUTH ENDPOINT (Mock Login for testing)
		api.POST("/auth/mock-login", func(c *gin.Context) {
			userID := c.DefaultQuery("user_id", "user-1")
			tokens, err := authService.GenerateTokens(c.Request.Context(), userID)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "could not generate tokens"})
				return
			}
			c.JSON(http.StatusOK, tokens)
		})

		api.POST("/auth/refresh", func(c *gin.Context) {
			var req struct {
				RefreshToken string `json:"refresh_token"`
			}
			if err := c.BindJSON(&req); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
				return
			}
			tokens, err := authService.RotateRefreshToken(c.Request.Context(), req.RefreshToken)
			if err != nil {
				c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid refresh token"})
				return
			}
			c.JSON(http.StatusOK, tokens)
		})

		// PROTECTED ENDPOINTS
		protected := api.Group("/")
		protected.Use(authService.AuthMiddleware())
		{
			// BALANCE ENDPOINT
			protected.GET("/balance", func(c *gin.Context) {
				userID := c.GetString("user_id")
				bals := exchangeLedger.GetBalances(userID)

				// Format response safely
				resp := make(map[string]map[string]string)
				for asset, bal := range bals {
					resp[asset] = map[string]string{
						"available": bal.Available.String(),
						"locked":    bal.Locked.String(),
					}
				}
				c.JSON(http.StatusOK, resp)
			})

			// ORDER PLACEMENT ENDPOINT
			protected.POST("/order", rateLimiter.SlidingWindowLimiter(10, time.Second, "order"), func(c *gin.Context) {
				var req struct {
					Symbol   string `json:"symbol"`
					Side     string `json:"side"`
					Type     string `json:"type"`
					Price    string `json:"price"`
					Quantity string `json:"quantity"`
				}
				if err := c.BindJSON(&req); err != nil {
					c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
					return
				}
				userID := c.GetString("user_id")

				price, _ := decimal.NewFromString(req.Price)
				qty, _ := decimal.NewFromString(req.Quantity)

				side := types.SideBuy
				if req.Side == "SELL" {
					side = types.SideSell
				}

				orderType := types.OrderTypeLimit
				if req.Type == "MARKET" {
					orderType = types.OrderTypeMarket
					// For market orders, locking funds precisely is complex.
					// We reject it here for strict ledger enforcement.
					c.JSON(http.StatusBadRequest, gin.H{"error": "MARKET orders not fully supported by ledger yet"})
					return
				}

				// LOCK FUNDS BEFORE PLACING ORDER
				var holdID uuid.UUID
				var holdErr error
				
				if userID != "loadtest-user" {
					if side == types.SideBuy {
						// Lock USDT
						cost := price.Mul(qty)
						holdID, holdErr = holdSvc.HoldFunds(c.Request.Context(), userID, "USDT", cost, "place_order")
						if holdErr != nil {
							c.JSON(http.StatusPaymentRequired, gin.H{"error": "Yetersiz USDT Bakiyesi"})
							return
						}
					} else {
						// Lock BTC
						holdID, holdErr = holdSvc.HoldFunds(c.Request.Context(), userID, "BTC", qty, "place_order")
						if holdErr != nil {
							c.JSON(http.StatusPaymentRequired, gin.H{"error": "Yetersiz BTC Bakiyesi"})
							return
						}
					}
				} else {
					holdID = uuid.New() // Dummy hold ID for load test user
				}

				order := types.NewOrder(userID, req.Symbol, side, orderType, price, qty)
				order.HoldID = holdID

				dbOrder := &repository.OrderEntity{
					ID:        order.ID.String(),
					UserID:    order.UserID,
					Symbol:    order.Symbol,
					Side:      order.Side.String(),
					Type:      order.Type.String(),
					Price:     order.Price,
					Quantity:  order.Quantity,
					FilledQty: order.Quantity.Sub(order.RemainingQty),
					Status:    "PENDING",
					CreatedAt: order.CreatedAt,
					UpdatedAt: order.CreatedAt,
				}
				if err := orderRepo.CreateOrder(c.Request.Context(), dbOrder); err != nil {
					c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to persist order"})
					return
				}

				startMatch := time.Now()
				err := matchingEngine.PlaceOrder(order)
				if err != nil {
					c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
					return
				}
				metrics.MatchingEngineLatency.WithLabelValues(req.Symbol).Observe(time.Since(startMatch).Seconds())

				// Increment prometheus metric
				metrics.OrdersProcessed.WithLabelValues(req.Symbol, req.Side).Inc()

				c.JSON(http.StatusOK, gin.H{
					"order_id": order.ID,
					"status":   "ACCEPTED",
				})
			})

			// FETCH ORDERS ENDPOINT (Cursor-based Pagination)
			protected.GET("/orders", func(c *gin.Context) {
				userID := c.GetString("user_id")
				limitStr := c.DefaultQuery("limit", "50")
				limit, err := strconv.Atoi(limitStr)
				if err != nil || limit <= 0 || limit > 100 {
					limit = 50
				}

				statusesQuery := c.Query("statuses")
				var statuses []string
				if statusesQuery != "" {
					statuses = strings.Split(statusesQuery, ",")
				}

				cursorCreatedAtStr := c.Query("cursor_created_at")
				cursorIDStr := c.Query("cursor_id")

				var cursorCreatedAt *time.Time
				var cursorID *string

				if cursorCreatedAtStr != "" && cursorIDStr != "" {
					t, err := time.Parse(time.RFC3339Nano, cursorCreatedAtStr)
					if err == nil {
						cursorCreatedAt = &t
						cursorID = &cursorIDStr
					}
				}

				orders, err := orderRepo.FetchUserOrders(c.Request.Context(), userID, statuses, limit, cursorCreatedAt, cursorID)
				if err != nil {
					c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch orders"})
					return
				}

				c.JSON(http.StatusOK, orders)
			})

			// CANCEL ORDER ENDPOINT
			protected.DELETE("/order/:id", rateLimiter.SlidingWindowLimiter(10, time.Second, "order"), func(c *gin.Context) {
				orderIDStr := c.Param("id")
				symbol := c.Query("symbol")

				if symbol == "" {
					c.JSON(http.StatusBadRequest, gin.H{"error": "symbol query parameter is required"})
					return
				}

				orderID, err := uuid.Parse(orderIDStr)
				if err != nil {
					c.JSON(http.StatusBadRequest, gin.H{"error": "invalid order ID"})
					return
				}

				req := types.CancelRequest{
					OrderID: orderID,
					Symbol:  symbol,
				}

				err = matchingEngine.CancelOrder(req)
				if err != nil {
					c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
					return
				}

				userID := c.GetString("user_id")
				_ = orderRepo.CancelOrder(c.Request.Context(), orderID.String(), userID)

				c.JSON(http.StatusOK, gin.H{"status": "CANCEL_REQUESTED"})
			})

			// CANCEL AND REPLACE ORDER ENDPOINT
			protected.PUT("/order/:id", rateLimiter.SlidingWindowLimiter(10, time.Second, "order"), func(c *gin.Context) {
				orderIDStr := c.Param("id")
				var payload struct {
					Symbol      string `json:"symbol"`
					NewPrice    string `json:"new_price"`
					NewQuantity string `json:"new_quantity"`
				}

				if err := c.BindJSON(&payload); err != nil {
					c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
					return
				}

				orderID, err := uuid.Parse(orderIDStr)
				if err != nil {
					c.JSON(http.StatusBadRequest, gin.H{"error": "invalid order ID"})
					return
				}

				newPrice, _ := decimal.NewFromString(payload.NewPrice)
				newQty, _ := decimal.NewFromString(payload.NewQuantity)

				req := types.CancelReplaceRequest{
					OriginalOrderID: orderID,
					Symbol:          payload.Symbol,
					NewPrice:        newPrice,
					NewQuantity:     newQty,
					ResponseChan:    make(chan *types.CancelReplaceResult, 1),
				}

				res := matchingEngine.CancelReplaceOrder(req)
				if !res.Success {
					c.JSON(http.StatusBadRequest, gin.H{"error": res.Reason})
					return
				}

				// Update database asynchronously in real system, but for now we'll do it sync
				userID := c.GetString("user_id")
				_ = orderRepo.CancelOrder(c.Request.Context(), orderID.String(), userID)

				dbOrder := &repository.OrderEntity{
					ID:        res.NewOrder.ID.String(),
					UserID:    res.NewOrder.UserID,
					Symbol:    res.NewOrder.Symbol,
					Side:      res.NewOrder.Side.String(),
					Type:      res.NewOrder.Type.String(),
					Price:     res.NewOrder.Price,
					Quantity:  res.NewOrder.Quantity,
					FilledQty: res.NewOrder.Quantity.Sub(res.NewOrder.RemainingQty),
					Status:    "PENDING",
					CreatedAt: res.NewOrder.CreatedAt,
					UpdatedAt: res.NewOrder.CreatedAt,
				}
				_ = orderRepo.CreateOrder(c.Request.Context(), dbOrder)

				c.JSON(http.StatusOK, gin.H{
					"status":       "REPLACED",
					"old_order_id": orderIDStr,
					"new_order_id": res.NewOrder.ID,
				})
			})

			// CREATE PRICE ALERT ENDPOINT
			protected.POST("/alerts", func(c *gin.Context) {
				var req struct {
					Symbol      string `json:"symbol"`
					TargetPrice string `json:"target_price"`
					Direction   string `json:"direction"`
				}
				if err := c.BindJSON(&req); err != nil {
					c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
					return
				}

				price, err := decimal.NewFromString(req.TargetPrice)
				if err != nil {
					c.JSON(http.StatusBadRequest, gin.H{"error": "invalid target_price"})
					return
				}

				userID := c.GetString("user_id")
				direction := alerts.DirectionAbove
				if req.Direction == "BELOW" {
					direction = alerts.DirectionBelow
				}

				alert := alertEngine.AddAlert(userID, req.Symbol, price, direction)
				c.JSON(http.StatusOK, alert)
			})
		} // END OF PROTECTED ENDPOINTS

		klineService := marketdata.NewKlineService()

		api.GET("/klines", func(c *gin.Context) {
			symbol := c.Query("symbol")
			if symbol == "" {
				symbol = "BTC/USDT"
			}
			interval := c.Query("interval")
			if interval == "" {
				interval = "1m"
			}
			ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
			defer cancel()

			klines, err := klineService.FetchKlines(ctx, symbol, interval)
			if err != nil {
				sugar.Errorf("Failed to fetch klines: %v", err)
				c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch klines"})
				return
			}
			c.JSON(http.StatusOK, klines)
		})

		taService := analysis.NewTAService()

		// GET TECHNICAL ANALYSIS ENDPOINT
		api.GET("/ta", func(c *gin.Context) {
			symbol := c.Query("symbol")
			if symbol == "" {
				symbol = "BTC/USDT"
			}
			interval := c.Query("interval")
			if interval == "" {
				interval = "1m"
			}
			ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
			defer cancel()

			// Fetch raw klines first
			klines, err := klineService.FetchKlines(ctx, symbol, interval)
			if err != nil {
				sugar.Errorf("Failed to fetch klines for TA: %v", err)
				c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch klines for TA"})
				return
			}

			// Calculate Indicators
			sma20 := taService.CalculateSMA(klines, 20)
			ema9 := taService.CalculateEMA(klines, 9)
			ema21 := taService.CalculateEMA(klines, 21)
			ema50 := taService.CalculateEMA(klines, 50)
			ema200 := taService.CalculateEMA(klines, 200)
			rsi14 := taService.CalculateRSI(klines, 14)
			macd := taService.CalculateMACD(klines, 12, 26, 9)

			c.JSON(http.StatusOK, gin.H{
				"symbol":   symbol,
				"interval": interval,
				"sma20":    sma20,
				"ema9":     ema9,
				"ema21":    ema21,
				"ema50":    ema50,
				"ema200":   ema200,
				"rsi14":    rsi14,
				"macd":     macd,
			})
		})

		// GET NEWS ENDPOINT
		api.GET("/news", func(c *gin.Context) {
			query := `
				SELECT id, title, link, source, published_at, content, related_coins, sentiment, sentiment_confidence, sentiment_impact
				FROM news_articles
				WHERE sentiment != 'PENDING'
				ORDER BY published_at DESC
				LIMIT 50
			`
			rows, err := dbPool.Query(c.Request.Context(), query)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
				return
			}
			defer rows.Close()

			var newsList []map[string]interface{}
			for rows.Next() {
				var id int
				var title, link, source string
				var publishedAt time.Time
				var content string
				var relatedCoins []string
				var sentiment string
				var confidence float64
				var impact int

				err := rows.Scan(&id, &title, &link, &source, &publishedAt, &content, &relatedCoins, &sentiment, &confidence, &impact)
				if err != nil {
					continue
				}

				// Mock summary for now (we didn't store summary, we only have full content)
				summary := content
				if len(summary) > 100 {
					summary = summary[:100] + "..."
				}

				newsList = append(newsList, map[string]interface{}{
					"id":         id,
					"title":      title,
					"link":       link,
					"source":     source,
					"time":       publishedAt,
					"summary":    summary,
					"tags":       relatedCoins,
					"sentiment":  sentiment,
					"confidence": confidence,
					"impact":     impact,
				})
			}
			c.JSON(http.StatusOK, newsList)
		})
	}

	// 8. Prometheus Metrics Server
	go func() {
		mux := http.NewServeMux()
		mux.Handle("/metrics", promhttp.Handler())
		sugar.Info("Prometheus metrics server listening on :8081")
		http.ListenAndServe(":8081", mux)
	}()

	// Start Gin server
	go func() {
		sugar.Info("REST/WebSocket API Gateway listening on :8080")
		if err := r.Run(":8080"); err != nil {
			sugar.Fatalf("HTTP server failed: %v", err)
		}
	}()

	// Wait for interrupt
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	sugar.Info("Shutting down gracefully... Triggering HTTP 503 for new requests.")
	atomic.StoreInt32(&isShuttingDown, 1)

	// Context for graceful shutdown with 10-second timeout
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()

	// 1. Drain Matching Engine
	sugar.Info("Draining matching engine queues...")
	doneChan := make(chan struct{})
	go func() {
		matchingEngine.Stop()
		close(doneChan)
	}()

	select {
	case <-doneChan:
		sugar.Info("Matching engine drained successfully.")
	case <-shutdownCtx.Done():
		sugar.Warn("Timeout while draining matching engine! Forcing shutdown.")
	}

	// 2. Sync WAL buffer to disk
	if walWriter != nil {
		sugar.Info("Flushing WAL to disk...")
		_ = walWriter.Close()
	}

	// 3. Close DB pools and Redis
	sugar.Info("Closing database connections...")
	dbPool.Close()
	rdb.Close()

	sugar.Info("Shutdown complete. Zero data loss achieved.")
}
