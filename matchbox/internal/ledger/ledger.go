package ledger

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shopspring/decimal"
)

var (
	ErrInsufficientFunds = errors.New("insufficient funds")
	ErrAccountNotFound   = errors.New("account not found")
)

type Ledger struct {
	db *pgxpool.Pool
}

func NewLedger(db *pgxpool.Pool) *Ledger {
	return &Ledger{db: db}
}

func (l *Ledger) getOrCreateAccount(ctx context.Context, tx pgx.Tx, code, name, acctType, currency string) (uuid.UUID, error) {
	var id uuid.UUID
	err := tx.QueryRow(ctx, `
		INSERT INTO accounts (code, name, type, currency)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (code) DO UPDATE SET name = EXCLUDED.name
		RETURNING id
	`, code, name, acctType, currency).Scan(&id)
	return id, err
}

func isConstraintViolation(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), "balance_non_negative")
}

func (l *Ledger) Deposit(userID, asset string, amount decimal.Decimal) {
	if amount.LessThanOrEqual(decimal.Zero) {
		return
	}
	ctx := context.Background()
	tx, err := l.db.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return
	}
	defer tx.Rollback(ctx)

	userCode := fmt.Sprintf("USER:%s:%s:AVAILABLE", userID, asset)
	sysCode := fmt.Sprintf("SYSTEM:HOT_WALLET:%s", asset)

	userIDAcct, err := l.getOrCreateAccount(ctx, tx, userCode, "User Available Balance", "LIABILITY", asset)
	if err != nil { return }
	sysIDAcct, err := l.getOrCreateAccount(ctx, tx, sysCode, "System Hot Wallet", "ASSET", asset)
	if err != nil { return }

	var jeID uuid.UUID
	err = tx.QueryRow(ctx, `INSERT INTO journal_entries (description, reference_id) VALUES ($1, $2) RETURNING id`,
		fmt.Sprintf("Deposit %s %s for %s", amount.String(), asset, userID), "SYS_DEPOSIT").Scan(&jeID)
	if err != nil { return }

	// Asset increases -> DEBIT
	_, err = tx.Exec(ctx, `INSERT INTO ledger_postings (journal_entry_id, account_id, amount, direction) VALUES ($1, $2, $3, 'DEBIT')`, jeID, sysIDAcct, amount)
	if err != nil { return }
	// Liability increases -> CREDIT
	_, err = tx.Exec(ctx, `INSERT INTO ledger_postings (journal_entry_id, account_id, amount, direction) VALUES ($1, $2, $3, 'CREDIT')`, jeID, userIDAcct, amount)
	if err != nil { return }

	_ = tx.Commit(ctx)
}

func (l *Ledger) Withdraw(userID, asset string, amount decimal.Decimal) error {
	if amount.LessThanOrEqual(decimal.Zero) {
		return errors.New("invalid withdrawal amount")
	}
	ctx := context.Background()
	var err error
	for attempts := 0; attempts < 10; attempts++ {
		err = l.attemptWithdraw(ctx, userID, asset, amount)
		if err == nil {
			return nil
		}
		if err == ErrInsufficientFunds || err == ErrAccountNotFound {
			return err
		}
		time.Sleep(time.Duration(rand.Intn(10)+5) * time.Millisecond)
	}
	return err
}

func (l *Ledger) attemptWithdraw(ctx context.Context, userID, asset string, amount decimal.Decimal) error {
	tx, err := l.db.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil { return err }
	defer tx.Rollback(ctx)

	userCode := fmt.Sprintf("USER:%s:%s:AVAILABLE", userID, asset)
	sysCode := fmt.Sprintf("SYSTEM:HOT_WALLET:%s", asset)

	userIDAcct, err := l.getOrCreateAccount(ctx, tx, userCode, "User Available Balance", "LIABILITY", asset)
	if err != nil { return err }
	sysIDAcct, err := l.getOrCreateAccount(ctx, tx, sysCode, "System Hot Wallet", "ASSET", asset)
	if err != nil { return err }

	var jeID uuid.UUID
	err = tx.QueryRow(ctx, `INSERT INTO journal_entries (description, reference_id) VALUES ($1, $2) RETURNING id`,
		fmt.Sprintf("Withdraw %s %s for %s", amount.String(), asset, userID), "SYS_WITHDRAW").Scan(&jeID)
	if err != nil { return err }

	// Liability decreases -> DEBIT
	_, err = tx.Exec(ctx, `INSERT INTO ledger_postings (journal_entry_id, account_id, amount, direction) VALUES ($1, $2, $3, 'DEBIT')`, jeID, userIDAcct, amount)
	if err != nil {
		if isConstraintViolation(err) { return ErrInsufficientFunds }
		return err
	}
	// Asset decreases -> CREDIT
	_, err = tx.Exec(ctx, `INSERT INTO ledger_postings (journal_entry_id, account_id, amount, direction) VALUES ($1, $2, $3, 'CREDIT')`, jeID, sysIDAcct, amount)
	if err != nil {
		if isConstraintViolation(err) { return ErrInsufficientFunds }
		return err
	}

	return tx.Commit(ctx)
}

func (l *Ledger) LockFunds(userID, asset string, amount decimal.Decimal) error {
	ctx := context.Background()
	var err error
	for attempts := 0; attempts < 10; attempts++ {
		err = l.attemptLockFunds(ctx, userID, asset, amount)
		if err == nil { return nil }
		if err == ErrInsufficientFunds || err == ErrAccountNotFound { return err }
		time.Sleep(time.Duration(rand.Intn(10)+5) * time.Millisecond)
	}
	return err
}

func (l *Ledger) attemptLockFunds(ctx context.Context, userID, asset string, amount decimal.Decimal) error {
	tx, err := l.db.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil { return err }
	defer tx.Rollback(ctx)

	availCode := fmt.Sprintf("USER:%s:%s:AVAILABLE", userID, asset)
	lockedCode := fmt.Sprintf("USER:%s:%s:LOCKED", userID, asset)

	availID, err := l.getOrCreateAccount(ctx, tx, availCode, "User Available Balance", "LIABILITY", asset)
	if err != nil { return err }
	lockedID, err := l.getOrCreateAccount(ctx, tx, lockedCode, "User Locked Balance", "LIABILITY", asset)
	if err != nil { return err }

	var jeID uuid.UUID
	err = tx.QueryRow(ctx, `INSERT INTO journal_entries (description, reference_id) VALUES ($1, $2) RETURNING id`,
		fmt.Sprintf("Lock %s %s for %s", amount.String(), asset, userID), "SYS_LOCK").Scan(&jeID)
	if err != nil { return err }

	// Decrease liability (Available) -> DEBIT
	_, err = tx.Exec(ctx, `INSERT INTO ledger_postings (journal_entry_id, account_id, amount, direction) VALUES ($1, $2, $3, 'DEBIT')`, jeID, availID, amount)
	if err != nil {
		if isConstraintViolation(err) { return ErrInsufficientFunds }
		return err
	}
	// Increase liability (Locked) -> CREDIT
	_, err = tx.Exec(ctx, `INSERT INTO ledger_postings (journal_entry_id, account_id, amount, direction) VALUES ($1, $2, $3, 'CREDIT')`, jeID, lockedID, amount)
	if err != nil { return err }

	return tx.Commit(ctx)
}

func (l *Ledger) UnlockFunds(userID, asset string, amount decimal.Decimal, idempotencyKey string) error {
	ctx := context.Background()
	var err error
	for attempts := 0; attempts < 10; attempts++ {
		err = l.attemptUnlockFunds(ctx, userID, asset, amount, idempotencyKey)
		if err == nil { return nil }
		if err == ErrAccountNotFound || err.Error() == "insufficient locked funds to unlock" || err.Error() == "duplicate cancel request" {
			return err
		}
		time.Sleep(time.Duration(rand.Intn(10)+5) * time.Millisecond)
	}
	return err
}

func (l *Ledger) attemptUnlockFunds(ctx context.Context, userID, asset string, amount decimal.Decimal, idempotencyKey string) error {
	tx, err := l.db.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil { return err }
	defer tx.Rollback(ctx)

	if idempotencyKey != "" {
		_, err = tx.Exec(ctx, `INSERT INTO idempotency_keys (key) VALUES ($1)`, idempotencyKey)
		if err != nil {
			if strings.Contains(err.Error(), "23505") || strings.Contains(err.Error(), "duplicate key value") {
				return errors.New("duplicate cancel request")
			}
			return errors.New("duplicate cancel request")
		}
	}

	availCode := fmt.Sprintf("USER:%s:%s:AVAILABLE", userID, asset)
	lockedCode := fmt.Sprintf("USER:%s:%s:LOCKED", userID, asset)

	availID, err := l.getOrCreateAccount(ctx, tx, availCode, "User Available Balance", "LIABILITY", asset)
	if err != nil { return err }
	lockedID, err := l.getOrCreateAccount(ctx, tx, lockedCode, "User Locked Balance", "LIABILITY", asset)
	if err != nil { return err }

	var jeID uuid.UUID
	err = tx.QueryRow(ctx, `INSERT INTO journal_entries (description, reference_id) VALUES ($1, $2) RETURNING id`,
		fmt.Sprintf("Unlock %s %s for %s", amount.String(), asset, userID), "SYS_UNLOCK").Scan(&jeID)
	if err != nil { return err }

	// Decrease liability (Locked) -> DEBIT
	_, err = tx.Exec(ctx, `INSERT INTO ledger_postings (journal_entry_id, account_id, amount, direction) VALUES ($1, $2, $3, 'DEBIT')`, jeID, lockedID, amount)
	if err != nil {
		if isConstraintViolation(err) { return errors.New("insufficient locked funds to unlock") }
		return err
	}
	// Increase liability (Available) -> CREDIT
	_, err = tx.Exec(ctx, `INSERT INTO ledger_postings (journal_entry_id, account_id, amount, direction) VALUES ($1, $2, $3, 'CREDIT')`, jeID, availID, amount)
	if err != nil { return err }

	return tx.Commit(ctx)
}

func (l *Ledger) SettleTrade(tradeID uuid.UUID, buyerID, sellerID string, baseAsset, quoteAsset string, price, quantity decimal.Decimal) error {
	var err error
	for attempts := 0; attempts < 10; attempts++ {
		err = l.attemptSettleTrade(tradeID, buyerID, sellerID, baseAsset, quoteAsset, price, quantity)
		if err == nil { return nil }
		if err == ErrInsufficientFunds || err.Error() == "buyer does not have enough locked quote funds" || err.Error() == "seller does not have enough locked base funds" {
			return err
		}
		time.Sleep(time.Duration(rand.Intn(20)+10) * time.Millisecond)
	}
	return err
}

func (l *Ledger) attemptSettleTrade(tradeID uuid.UUID, buyerID, sellerID string, baseAsset, quoteAsset string, price, quantity decimal.Decimal) error {
	quoteAmount := price.Mul(quantity)
	baseAmount := quantity

	ctx := context.Background()
	tx, err := l.db.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil { return err }
	defer tx.Rollback(ctx)

	// Get or Create accounts
	buyerQuoteLockedID, err := l.getOrCreateAccount(ctx, tx, fmt.Sprintf("USER:%s:%s:LOCKED", buyerID, quoteAsset), "Buyer Quote Locked", "LIABILITY", quoteAsset)
	if err != nil { return err }
	sellerQuoteAvailID, err := l.getOrCreateAccount(ctx, tx, fmt.Sprintf("USER:%s:%s:AVAILABLE", sellerID, quoteAsset), "Seller Quote Available", "LIABILITY", quoteAsset)
	if err != nil { return err }

	sellerBaseLockedID, err := l.getOrCreateAccount(ctx, tx, fmt.Sprintf("USER:%s:%s:LOCKED", sellerID, baseAsset), "Seller Base Locked", "LIABILITY", baseAsset)
	if err != nil { return err }
	buyerBaseAvailID, err := l.getOrCreateAccount(ctx, tx, fmt.Sprintf("USER:%s:%s:AVAILABLE", buyerID, baseAsset), "Buyer Base Available", "LIABILITY", baseAsset)
	if err != nil { return err }

	// Create Journal Entry
	var jeID uuid.UUID
	err = tx.QueryRow(ctx, `INSERT INTO journal_entries (description, reference_id) VALUES ($1, $2) RETURNING id`,
		fmt.Sprintf("Trade Settlement %s", tradeID.String()), "TRADE:"+tradeID.String()).Scan(&jeID)
	if err != nil { return err }

	// 1. Move Quote from Buyer Locked to Seller Available
	// Decrease Buyer Locked Liability -> DEBIT
	_, err = tx.Exec(ctx, `INSERT INTO ledger_postings (journal_entry_id, account_id, amount, direction) VALUES ($1, $2, $3, 'DEBIT')`, jeID, buyerQuoteLockedID, quoteAmount)
	if err != nil {
		if isConstraintViolation(err) { return errors.New("buyer does not have enough locked quote funds") }
		return err
	}
	// Increase Seller Available Liability -> CREDIT
	_, err = tx.Exec(ctx, `INSERT INTO ledger_postings (journal_entry_id, account_id, amount, direction) VALUES ($1, $2, $3, 'CREDIT')`, jeID, sellerQuoteAvailID, quoteAmount)
	if err != nil { return err }

	// 2. Move Base from Seller Locked to Buyer Available
	// Decrease Seller Locked Liability -> DEBIT
	_, err = tx.Exec(ctx, `INSERT INTO ledger_postings (journal_entry_id, account_id, amount, direction) VALUES ($1, $2, $3, 'DEBIT')`, jeID, sellerBaseLockedID, baseAmount)
	if err != nil {
		if isConstraintViolation(err) { return errors.New("seller does not have enough locked base funds") }
		return err
	}
	// Increase Buyer Available Liability -> CREDIT
	_, err = tx.Exec(ctx, `INSERT INTO ledger_postings (journal_entry_id, account_id, amount, direction) VALUES ($1, $2, $3, 'CREDIT')`, jeID, buyerBaseAvailID, baseAmount)
	if err != nil { return err }

	return tx.Commit(ctx)
}

type Balance struct {
	Available decimal.Decimal
	Locked    decimal.Decimal
}

func (l *Ledger) GetBalances(userID string) map[string]*Balance {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	rows, err := l.db.Query(ctx, `SELECT currency, code, balance FROM accounts WHERE code LIKE 'USER:' || $1 || ':%'`, userID)
	if err != nil {
		return nil
	}
	defer rows.Close()

	bals := make(map[string]*Balance)
	for rows.Next() {
		var currency, code string
		var balance decimal.Decimal
		if err := rows.Scan(&currency, &code, &balance); err != nil {
			continue
		}
		if bals[currency] == nil {
			bals[currency] = &Balance{}
		}
		if strings.HasSuffix(code, ":AVAILABLE") {
			bals[currency].Available = balance
		} else if strings.HasSuffix(code, ":LOCKED") {
			bals[currency].Locked = balance
		}
	}
	return bals
}
