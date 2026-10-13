package ledger

import (
	"context"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
	"github.com/shopspring/decimal"
)

// setupTestServices initializes db and redis for integration testing
func setupTestServices(t *testing.T) (*pgxpool.Pool, *redis.Client, *HoldService, *Ledger) {
	ctx := context.Background()

	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		dbURL = "postgres://postgres:postgres@localhost:5432/matchbox?sslmode=disable"
	}
	dbPool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		t.Fatalf("Failed to connect to DB: %v", err)
	}

	redisURL := os.Getenv("REDIS_URL")
	if redisURL == "" {
		redisURL = "redis://localhost:6379/0"
	}
	opt, _ := redis.ParseURL(redisURL)
	rdb := redis.NewClient(opt)

	transferSvc := NewLedgerTransferService(dbPool, rdb)
	holdSvc := NewHoldService(dbPool, transferSvc)
	legacyLedger := NewLedger(dbPool) // for initial deposit

	// Make sure holds table is ready
	err = holdSvc.InitSchema(ctx)
	if err != nil {
		t.Fatalf("Failed to init schema: %v", err)
	}

	return dbPool, rdb, holdSvc, legacyLedger
}

func TestHoldService_RaceConditions(t *testing.T) {
	dbPool, rdb, holdSvc, legacyLedger := setupTestServices(t)
	defer dbPool.Close()
	defer rdb.Close()
	ctx := context.Background()

	userID := "race_test_user_" + time.Now().String()
	asset := "USDT"

	// 1. Give user exactly 100 USDT
	initialAmount := decimal.NewFromInt(100)
	legacyLedger.Deposit(userID, asset, initialAmount)

	// --- RACE CONDITION TEST 1: Concurrent Holds ---
	// We will try to place 10 holds of 20 USDT simultaneously.
	// Exactly 5 should succeed, and 5 should fail with ErrInsufficientFunds or serialization failure.
	
	t.Run("Concurrent HoldFunds", func(t *testing.T) {
		var wg sync.WaitGroup
		concurrency := 10
		holdAmount := decimal.NewFromInt(20)

		var successCount int32
		var failCount int32

		for i := 0; i < concurrency; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				_, err := holdSvc.HoldFunds(ctx, userID, asset, holdAmount, "Test Race Hold")
				if err == nil {
					atomic.AddInt32(&successCount, 1)
				} else {
					atomic.AddInt32(&failCount, 1)
				}
			}()
		}
		wg.Wait()

		if successCount != 5 {
			t.Errorf("Expected exactly 5 successful holds out of 100 USDT (20 USDT each), got %d successes and %d failures", successCount, failCount)
		}
	})

	// --- RACE CONDITION TEST 2: Concurrent Commit vs Release ---
	t.Run("Concurrent Commit and Release", func(t *testing.T) {
		// Give user more funds for this test
		legacyLedger.Deposit(userID, asset, decimal.NewFromInt(50))
		holdID, err := holdSvc.HoldFunds(ctx, userID, asset, decimal.NewFromInt(50), "Test Commit/Release Race")
		if err != nil {
			t.Fatalf("Failed to create hold: %v", err)
		}

		var wg sync.WaitGroup
		wg.Add(2)

		var commitErr error
		var releaseErr error

		go func() {
			defer wg.Done()
			commitErr = holdSvc.CommitHold(ctx, holdID, TradeSettlementPayload{
				CounterpartyID: "counterparty_1",
				Asset:          asset,
				Amount:         decimal.NewFromInt(50),
			})
		}()

		go func() {
			defer wg.Done()
			releaseErr = holdSvc.ReleaseHold(ctx, holdID)
		}()

		wg.Wait()

		// Exactly one MUST succeed, and the other MUST fail with ErrHoldAlreadyProcessed (or serialization error)
		if commitErr == nil && releaseErr == nil {
			t.Fatal("CRITICAL RACE CONDITION: Both Commit and Release succeeded on the same Hold ID!")
		}
		
		if commitErr != nil && releaseErr != nil {
			t.Fatalf("Both operations failed! CommitErr: %v, ReleaseErr: %v", commitErr, releaseErr)
		}
	})
}
