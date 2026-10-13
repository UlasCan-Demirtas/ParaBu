package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"hookstorm/internal/dispatcher"
	"hookstorm/internal/dlq"
	"hookstorm/internal/ratelimit"
	"hookstorm/internal/worker"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/nats-io/nats.go"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

func main() {
	logger, _ := zap.NewDevelopment()
	sugar := logger.Sugar()
	defer logger.Sync()

	sugar.Info("Starting Hookstorm Webhook Gateway...")

	// 1. Connect to NATS
	nc, err := nats.Connect(nats.DefaultURL)
	if err != nil {
		sugar.Errorf("\n\n⚠️  KRİTİK HATA: NATS Sunucusuna Bağlanılamadı! ⚠️\n\nHookstorm bir 'Mesaj Kuyruğu Dinleyicisi' olduğu için çalışabilmesi adına NATS sunucusuna ihtiyaç duyar.\nLütfen Mac'inizde 'Docker Desktop' uygulamasını açıp 'docker-compose up -d' komutunu çalıştırdığınızdan emin olun.\n\nHata Detayı: %v\n\n", err)
		os.Exit(1)
	}
	defer nc.Close()

	// 2. Connect to Redis (for Rate Limiter)
	rdb := redis.NewClient(&redis.Options{
		Addr: "localhost:6379",
	})
	if err := rdb.Ping(context.Background()).Err(); err != nil {
		sugar.Warnf("Redis not available, rate limiter will fail: %v", err)
	}

	// 3. Connect to Postgres (for DLQ)
	dbPool, err := pgxpool.New(context.Background(), "postgres://parabu:parabu@localhost:5432/matchbox?sslmode=disable")
	if err != nil {
		sugar.Warnf("Postgres not available, DLQ will fail: %v", err)
	} else {
		defer dbPool.Close()
		// Auto-migrate DLQ table
		dbPool.Exec(context.Background(), `
			CREATE TABLE IF NOT EXISTS dead_letter_queue (
				id SERIAL PRIMARY KEY,
				endpoint VARCHAR(255) NOT NULL,
				payload TEXT NOT NULL,
				failure_reason TEXT NOT NULL,
				created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
			)
		`)
	}

	// 4. Initialize Components
	pool := worker.NewPool(20) // 20 concurrent workers
	limiter := ratelimit.NewLimiter(rdb, sugar)
	dlqStore := dlq.NewStore(dbPool, sugar)

	// In a real app, endpoints and secrets would come from a database.
	webhookEndpoint := "http://localhost:8080/webhook-receiver"
	webhookSecret := "parabu_super_secret"

	disp := dispatcher.NewDispatcher(nc, sugar, pool, limiter, dlqStore, webhookSecret, webhookEndpoint)
	
	err = disp.Start("matchbox.events.>")
	if err != nil {
		sugar.Fatalf("Failed to start dispatcher: %v", err)
	}

	// Graceful Shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	sugar.Info("Shutting down Hookstorm...")
	disp.Stop()
	pool.Stop()
	sugar.Info("Shutdown complete.")
}
