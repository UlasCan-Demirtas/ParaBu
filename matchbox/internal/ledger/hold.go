package ledger

import (
	"context"
	"errors"
	"fmt"

	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shopspring/decimal"
)

var (
	ErrHoldAlreadyProcessed = errors.New("hold already committed or released")
)

type TradeSettlementPayload struct {
	CounterpartyID string
	Asset          string
	Amount         decimal.Decimal // Miktar hold edilenle aynı veya daha az olabilir (kısmi)
}

// HoldService implements the 2-Phase Commit pattern for fund reservations.
type HoldService struct {
	db          *pgxpool.Pool
	transferSvc *LedgerTransferService
}

func NewHoldService(db *pgxpool.Pool, transferSvc *LedgerTransferService) *HoldService {
	return &HoldService{
		db:          db,
		transferSvc: transferSvc,
	}
}

// Ensure holds table exists. This is typically done in migrations, but we run it idempotently here for completeness.
func (s *HoldService) InitSchema(ctx context.Context) error {
	_, err := s.db.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS holds (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			user_id VARCHAR(100) NOT NULL,
			asset VARCHAR(20) NOT NULL,
			amount DECIMAL(32, 18) NOT NULL,
			status VARCHAR(20) NOT NULL DEFAULT 'ACTIVE', -- ACTIVE, COMMITTED, RELEASED
			created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
		);
	`)
	return err
}

// HoldFunds uses a ROW-LEVEL LOCK (SELECT FOR UPDATE) to lock funds synchronously, guaranteeing no overdraft.
func (s *HoldService) HoldFunds(ctx context.Context, userID, asset string, amount decimal.Decimal, reason string) (uuid.UUID, error) {
	var holdID uuid.UUID
	var txErr error
	maxRetries := 5
	baseDelay := 10 * time.Millisecond

	for i := 0; i < maxRetries; i++ {
		holdID, txErr = s.attemptHoldFunds(ctx, userID, asset, amount, reason)
		if txErr == nil {
			return holdID, nil
		}
		if strings.Contains(txErr.Error(), "40001") || strings.Contains(strings.ToLower(txErr.Error()), "serialization") {
			delay := baseDelay * (1 << i)
			jitter := time.Duration(time.Now().UnixNano()%int64(delay)) / 2
			time.Sleep(delay + jitter)
			continue
		}
		return uuid.Nil, txErr
	}
	return uuid.Nil, fmt.Errorf("hold failed after %d retries: %w", maxRetries, txErr)
}

func (s *HoldService) attemptHoldFunds(ctx context.Context, userID, asset string, amount decimal.Decimal, reason string) (uuid.UUID, error) {
	tx, err := s.db.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return uuid.Nil, err
	}
	defer tx.Rollback(ctx)

	availCode := fmt.Sprintf("USER:%s:%s:AVAILABLE", userID, asset)
	lockedCode := fmt.Sprintf("USER:%s:%s:LOCKED", userID, asset)

	availID, err := s.transferSvc.getOrCreateAccount(ctx, tx, availCode, "Available", "LIABILITY", asset)
	if err != nil {
		return uuid.Nil, err
	}
	lockedID, err := s.transferSvc.getOrCreateAccount(ctx, tx, lockedCode, "Locked", "LIABILITY", asset)
	if err != nil {
		return uuid.Nil, err
	}

	// ROW-LEVEL LOCK: Prevent race conditions exactly here!
	var currentBalance decimal.Decimal
	err = tx.QueryRow(ctx, `SELECT balance FROM accounts WHERE id = $1 FOR UPDATE`, availID).Scan(&currentBalance)
	if err != nil {
		return uuid.Nil, err
	}
	if currentBalance.LessThan(amount) {
		return uuid.Nil, ErrInsufficientFunds
	}

	// Record the hold in our tracking table
	var holdID uuid.UUID
	err = tx.QueryRow(ctx, `
		INSERT INTO holds (user_id, asset, amount, status) 
		VALUES ($1, $2, $3, 'ACTIVE') RETURNING id
	`, userID, asset, amount).Scan(&holdID)
	if err != nil {
		return uuid.Nil, err
	}

	// Create Journal Entry
	var jeID uuid.UUID
	err = tx.QueryRow(ctx, `
		INSERT INTO journal_entries (description, reference_id) 
		VALUES ($1, $2) RETURNING id
	`, fmt.Sprintf("HOLD FUNDS: %s", reason), holdID.String()).Scan(&jeID)
	if err != nil {
		return uuid.Nil, err
	}

	// Move from AVAILABLE (Debit liability) to LOCKED (Credit liability)
	_, err = tx.Exec(ctx, `INSERT INTO ledger_postings (journal_entry_id, account_id, amount, direction) VALUES ($1, $2, $3, 'DEBIT')`, jeID, availID, amount)
	if err != nil {
		return uuid.Nil, err
	}
	_, err = tx.Exec(ctx, `INSERT INTO ledger_postings (journal_entry_id, account_id, amount, direction) VALUES ($1, $2, $3, 'CREDIT')`, jeID, lockedID, amount)
	if err != nil {
		return uuid.Nil, err
	}

	if err := tx.Commit(ctx); err != nil {
		return uuid.Nil, err
	}
	return holdID, nil
}

// CommitHold officially transfers the held funds to the counterparty.
func (s *HoldService) CommitHold(ctx context.Context, holdID uuid.UUID, payload TradeSettlementPayload) error {
	var txErr error
	maxRetries := 5
	baseDelay := 10 * time.Millisecond

	for i := 0; i < maxRetries; i++ {
		txErr = s.attemptCommitHold(ctx, holdID, payload)
		if txErr == nil {
			return nil
		}
		if strings.Contains(txErr.Error(), "40001") || strings.Contains(strings.ToLower(txErr.Error()), "serialization") {
			delay := baseDelay * (1 << i)
			jitter := time.Duration(time.Now().UnixNano()%int64(delay)) / 2
			time.Sleep(delay + jitter)
			continue
		}
		return txErr
	}
	return fmt.Errorf("commit failed after %d retries: %w", maxRetries, txErr)
}

func (s *HoldService) attemptCommitHold(ctx context.Context, holdID uuid.UUID, payload TradeSettlementPayload) error {
	tx, err := s.db.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	var userID, asset string
	var holdAmount decimal.Decimal
	var status string

	// FOR UPDATE ensures no race condition if ReleaseHold is called concurrently
	err = tx.QueryRow(ctx, `SELECT user_id, asset, amount, status FROM holds WHERE id = $1 FOR UPDATE`, holdID).Scan(&userID, &asset, &holdAmount, &status)
	if err != nil {
		return err
	}

	if status != "ACTIVE" {
		return ErrHoldAlreadyProcessed
	}

	if payload.Amount.GreaterThan(holdAmount) {
		return errors.New("cannot commit more than held amount")
	}

	newHoldAmount := holdAmount.Sub(payload.Amount)
	newStatus := "ACTIVE"
	if newHoldAmount.IsZero() {
		newStatus = "COMMITTED"
	}

	// Update Hold Status and Amount
	_, err = tx.Exec(ctx, `UPDATE holds SET amount = $1, status = $2 WHERE id = $3`, newHoldAmount, newStatus, holdID)
	if err != nil {
		return err
	}

	sellerLockedCode := fmt.Sprintf("USER:%s:%s:LOCKED", userID, asset)
	buyerAvailCode := fmt.Sprintf("USER:%s:%s:AVAILABLE", payload.CounterpartyID, asset)

	sellerLockedID, err := s.transferSvc.getOrCreateAccount(ctx, tx, sellerLockedCode, "Locked", "LIABILITY", asset)
	if err != nil { return err }
	
	buyerAvailID, err := s.transferSvc.getOrCreateAccount(ctx, tx, buyerAvailCode, "Available", "LIABILITY", asset)
	if err != nil { return err }

	// Create Journal Entry
	var jeID uuid.UUID
	err = tx.QueryRow(ctx, `
		INSERT INTO journal_entries (description, reference_id) 
		VALUES ($1, $2) RETURNING id
	`, fmt.Sprintf("COMMIT HOLD: %s to %s", holdID, payload.CounterpartyID), "COMMIT:"+holdID.String()).Scan(&jeID)
	if err != nil { return err }

	// Move from Seller LOCKED (Debit liability) to Buyer AVAILABLE (Credit liability)
	_, err = tx.Exec(ctx, `INSERT INTO ledger_postings (journal_entry_id, account_id, amount, direction) VALUES ($1, $2, $3, 'DEBIT')`, jeID, sellerLockedID, payload.Amount)
	if err != nil { return err }
	
	_, err = tx.Exec(ctx, `INSERT INTO ledger_postings (journal_entry_id, account_id, amount, direction) VALUES ($1, $2, $3, 'CREDIT')`, jeID, buyerAvailID, payload.Amount)
	if err != nil { return err }

	return tx.Commit(ctx)
}

// ReleaseHold cancels the hold and returns funds to available balance.
func (s *HoldService) ReleaseHold(ctx context.Context, holdID uuid.UUID) error {
	var txErr error
	maxRetries := 5
	baseDelay := 10 * time.Millisecond

	for i := 0; i < maxRetries; i++ {
		txErr = s.attemptReleaseHold(ctx, holdID)
		if txErr == nil {
			return nil
		}
		if strings.Contains(txErr.Error(), "40001") || strings.Contains(strings.ToLower(txErr.Error()), "serialization") {
			delay := baseDelay * (1 << i)
			jitter := time.Duration(time.Now().UnixNano()%int64(delay)) / 2
			time.Sleep(delay + jitter)
			continue
		}
		return txErr
	}
	return fmt.Errorf("release failed after %d retries: %w", maxRetries, txErr)
}

func (s *HoldService) attemptReleaseHold(ctx context.Context, holdID uuid.UUID) error {
	tx, err := s.db.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	var userID, asset string
	var holdAmount decimal.Decimal
	var status string

	// FOR UPDATE
	err = tx.QueryRow(ctx, `SELECT user_id, asset, amount, status FROM holds WHERE id = $1 FOR UPDATE`, holdID).Scan(&userID, &asset, &holdAmount, &status)
	if err != nil {
		return err
	}
	if status != "ACTIVE" {
		return ErrHoldAlreadyProcessed
	}

	// Update Status
	_, err = tx.Exec(ctx, `UPDATE holds SET status = 'RELEASED' WHERE id = $1`, holdID)
	if err != nil {
		return err
	}

	availCode := fmt.Sprintf("USER:%s:%s:AVAILABLE", userID, asset)
	lockedCode := fmt.Sprintf("USER:%s:%s:LOCKED", userID, asset)

	availID, _ := s.transferSvc.getOrCreateAccount(ctx, tx, availCode, "Available", "LIABILITY", asset)
	lockedID, _ := s.transferSvc.getOrCreateAccount(ctx, tx, lockedCode, "Locked", "LIABILITY", asset)

	var jeID uuid.UUID
	err = tx.QueryRow(ctx, `
		INSERT INTO journal_entries (description, reference_id) 
		VALUES ($1, $2) RETURNING id
	`, fmt.Sprintf("RELEASE HOLD: %s", holdID), "RELEASE:"+holdID.String()).Scan(&jeID)
	if err != nil { return err }

	// Debit LOCKED, Credit AVAILABLE
	_, err = tx.Exec(ctx, `INSERT INTO ledger_postings (journal_entry_id, account_id, amount, direction) VALUES ($1, $2, $3, 'DEBIT')`, jeID, lockedID, holdAmount)
	if err != nil { return err }
	
	_, err = tx.Exec(ctx, `INSERT INTO ledger_postings (journal_entry_id, account_id, amount, direction) VALUES ($1, $2, $3, 'CREDIT')`, jeID, availID, holdAmount)
	if err != nil { return err }

	return tx.Commit(ctx)
}
