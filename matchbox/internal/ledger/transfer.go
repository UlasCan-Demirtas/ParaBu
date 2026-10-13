package ledger

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
	"github.com/shopspring/decimal"
)

type TransferType string

const (
	TransferDeposit    TransferType = "DEPOSIT"
	TransferWithdrawal TransferType = "WITHDRAWAL"
	TransferTrade      TransferType = "TRADE"
	TransferInternal   TransferType = "INTERNAL"
)

type TransferRequest struct {
	IdempotencyKey uuid.UUID
	Type           TransferType
	ReferenceID    string
	Asset          string
	Amount         decimal.Decimal
	Description    string

	DebitAccountCode string
	DebitAccountName string
	DebitAccountType string // "ASSET", "LIABILITY", "EQUITY", "REVENUE", "EXPENSE"

	CreditAccountCode string
	CreditAccountName string
	CreditAccountType string // "ASSET", "LIABILITY", "EQUITY", "REVENUE", "EXPENSE"
}

type TransferResult struct {
	JournalEntryID uuid.UUID       `json:"journal_entry_id"`
	Status         string          `json:"status"`
	Timestamp      time.Time       `json:"timestamp"`
	Amount         decimal.Decimal `json:"amount"`
	ReferenceID    string          `json:"reference_id"`
}

// LedgerTransferService orchestrates strictly ACID, double-entry transfers
// with robust Idempotency and Serialization Failure (40001) handling.
type LedgerTransferService struct {
	db  *pgxpool.Pool
	rdb *redis.Client
}

func NewLedgerTransferService(db *pgxpool.Pool, rdb *redis.Client) *LedgerTransferService {
	return &LedgerTransferService{db: db, rdb: rdb}
}

// ExecuteTransfer applies a transfer with an Exponential Backoff retry loop
// tailored for PostgreSQL's SERIALIZABLE isolation anomalies (SQLSTATE 40001).
func (s *LedgerTransferService) ExecuteTransfer(ctx context.Context, req TransferRequest) (*TransferResult, error) {
	if req.Amount.LessThanOrEqual(decimal.Zero) {
		return nil, errors.New("transfer amount must be strictly positive")
	}

	// 1. O(1) Idempotency Check (No DB touch!)
	cacheKey := "idempotency:transfer:" + req.IdempotencyKey.String()
	val, err := s.rdb.Get(ctx, cacheKey).Result()
	if err == nil && val != "" {
		var cachedResult TransferResult
		if err := json.Unmarshal([]byte(val), &cachedResult); err == nil {
			return &cachedResult, nil // O(1) return
		}
	} else if err != redis.Nil {
		return nil, fmt.Errorf("redis idempotency check failed: %w", err)
	}

	// 2. Exponential Backoff Retry Loop for 40001 Serialization Failures
	maxRetries := 5
	baseDelay := 10 * time.Millisecond

	var result *TransferResult
	var txErr error

	for i := 0; i < maxRetries; i++ {
		result, txErr = s.attemptTransfer(ctx, req)
		if txErr == nil {
			break // Transaction committed perfectly
		}

		// Detect PostgreSQL Serialization Failure (SQLSTATE 40001)
		errStr := txErr.Error()
		if strings.Contains(errStr, "40001") || strings.Contains(strings.ToLower(errStr), "serialization") {
			// Calculate backoff with jitter to prevent thundering herd
			delay := baseDelay * (1 << i) // 10ms, 20ms, 40ms, 80ms, 160ms
			jitter := time.Duration(time.Now().UnixNano()%int64(delay)) / 2
			time.Sleep(delay + jitter)
			continue
		}

		// If it's a business logic error (e.g. balance_non_negative constraint violation), DO NOT RETRY.
		if isConstraintViolation(txErr) {
			return nil, ErrInsufficientFunds
		}
		
		return nil, txErr
	}

	if txErr != nil {
		return nil, fmt.Errorf("transfer failed after %d retries. Last error: %w", maxRetries, txErr)
	}

	// 3. Persist the success result to Redis for future idempotency checks
	resultJSON, _ := json.Marshal(result)
	s.rdb.Set(ctx, cacheKey, resultJSON, 7*24*time.Hour) // Retain idempotency cache for 7 days

	return result, nil
}

func (s *LedgerTransferService) attemptTransfer(ctx context.Context, req TransferRequest) (*TransferResult, error) {
	// Guaranteeing SERIALIZABLE isolation level
	tx, err := s.db.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	// Step 1: Ensure both accounts exist, or create them dynamically
	debitAcctID, err := s.getOrCreateAccount(ctx, tx, req.DebitAccountCode, req.DebitAccountName, req.DebitAccountType, req.Asset)
	if err != nil {
		return nil, err
	}
	
	creditAcctID, err := s.getOrCreateAccount(ctx, tx, req.CreditAccountCode, req.CreditAccountName, req.CreditAccountType, req.Asset)
	if err != nil {
		return nil, err
	}

	// Step 2: Create the Journal Entry
	var jeID uuid.UUID
	err = tx.QueryRow(ctx, `
		INSERT INTO journal_entries (description, reference_id) 
		VALUES ($1, $2) RETURNING id
	`, req.Description, req.ReferenceID).Scan(&jeID)
	
	if err != nil {
		return nil, err
	}

	// Step 3: Ledger Postings
	_, err = tx.Exec(ctx, `
		INSERT INTO ledger_postings (journal_entry_id, account_id, amount, direction) 
		VALUES ($1, $2, $3, 'DEBIT')
	`, jeID, debitAcctID, req.Amount)
	if err != nil {
		return nil, err
	}

	_, err = tx.Exec(ctx, `
		INSERT INTO ledger_postings (journal_entry_id, account_id, amount, direction) 
		VALUES ($1, $2, $3, 'CREDIT')
	`, jeID, creditAcctID, req.Amount)
	if err != nil {
		return nil, err
	}

	// Commit will trigger 'trg_check_double_entry' DEFERRED trigger!
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}

	return &TransferResult{
		JournalEntryID: jeID,
		Status:         "COMPLETED",
		Timestamp:      time.Now(),
		Amount:         req.Amount,
		ReferenceID:    req.ReferenceID,
	}, nil
}

func (s *LedgerTransferService) getOrCreateAccount(ctx context.Context, tx pgx.Tx, code, name, acctType, currency string) (uuid.UUID, error) {
	var id uuid.UUID
	err := tx.QueryRow(ctx, `
		INSERT INTO accounts (code, name, type, currency)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (code) DO UPDATE SET name = EXCLUDED.name
		RETURNING id
	`, code, name, acctType, currency).Scan(&id)
	return id, err
}
