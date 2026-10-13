package ledger

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
	"github.com/robfig/cron/v3"
	"go.uber.org/zap"
)

// ReconciliationWorker periodically audits the double-entry bookkeeping system.
// It ensures that SUM(Debit) == SUM(Credit) globally, and that the fundamental
// accounting equation (Assets + Expenses = Liabilities + Equity + Revenue) holds.
// If any discrepancy is found, it triggers a Circuit Breaker via Redis.
type ReconciliationWorker struct {
	db     *pgxpool.Pool
	redis  *redis.Client
	logger *zap.SugaredLogger
	cron   *cron.Cron
}

func NewReconciliationWorker(db *pgxpool.Pool, redisClient *redis.Client, logger *zap.SugaredLogger) *ReconciliationWorker {
	return &ReconciliationWorker{
		db:     db,
		redis:  redisClient,
		logger: logger,
		cron:   cron.New(),
	}
}

func (w *ReconciliationWorker) Start() {
	w.logger.Info("Starting ReconciliationWorker (Audit Daemon)...")

	// Run every hour at minute 0: "0 * * * *"
	// For testing purposes, you could change this to "* * * * *" (every minute)
	_, err := w.cron.AddFunc("0 * * * *", w.RunAudit)
	if err != nil {
		w.logger.Fatalf("Failed to schedule reconciliation cron: %v", err)
	}

	w.cron.Start()
}

func (w *ReconciliationWorker) Stop() {
	w.cron.Stop()
}

// RunAudit performs the mathematical equivalence checks on the ledger.
func (w *ReconciliationWorker) RunAudit() {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	w.logger.Info("Executing Scheduled Ledger Reconciliation Audit...")

	// CHECK 1: Double-Entry Equivalence on Postings (Global DEBIT == CREDIT)
	var totalDebit, totalCredit float64
	err := w.db.QueryRow(ctx, `
		SELECT 
			COALESCE(SUM(CASE WHEN direction = 'DEBIT' THEN amount ELSE 0 END), 0) as total_debit,
			COALESCE(SUM(CASE WHEN direction = 'CREDIT' THEN amount ELSE 0 END), 0) as total_credit
		FROM ledger_postings
	`).Scan(&totalDebit, &totalCredit)
	
	if err != nil {
		w.logger.Errorf("Audit failed while checking ledger_postings: %v", err)
		return
	}

	if totalDebit != totalCredit {
		w.triggerCircuitBreaker(ctx, fmt.Sprintf("CRITICAL DISCREPANCY: Global Debit (%.8f) != Global Credit (%.8f)", totalDebit, totalCredit))
		return
	}

	// CHECK 2: Fundamental Accounting Equation Per Currency
	// Asset + Expense = Liability + Equity + Revenue
	rows, err := w.db.Query(ctx, `
		SELECT 
			currency,
			COALESCE(SUM(CASE WHEN type IN ('ASSET', 'EXPENSE') THEN balance ELSE 0 END), 0) as left_side,
			COALESCE(SUM(CASE WHEN type IN ('LIABILITY', 'EQUITY', 'REVENUE') THEN balance ELSE 0 END), 0) as right_side
		FROM accounts
		GROUP BY currency
	`)
	if err != nil {
		w.logger.Errorf("Audit failed while querying accounts: %v", err)
		return
	}
	defer rows.Close()

	for rows.Next() {
		var currency string
		var leftSide, rightSide float64
		if err := rows.Scan(&currency, &leftSide, &rightSide); err != nil {
			w.logger.Errorf("Audit row scan failed: %v", err)
			continue
		}

		if leftSide != rightSide {
			msg := fmt.Sprintf("CRITICAL DISCREPANCY in %s: (Asset+Expense=%.8f) != (Liability+Equity+Revenue=%.8f)", currency, leftSide, rightSide)
			w.triggerCircuitBreaker(ctx, msg)
			return
		}
	}

	w.logger.Info("Ledger Reconciliation Audit PASSED successfully. System is mathematically sound.")
}

// triggerCircuitBreaker raises an alert and freezes withdrawals.
func (w *ReconciliationWorker) triggerCircuitBreaker(ctx context.Context, reason string) {
	w.logger.Errorf("CIRCUIT BREAKER TRIGGERED: %s", reason)

	// Set a flag in Redis that withdrawals are suspended
	err := w.redis.Set(ctx, "SYSTEM:CIRCUIT_BREAKER:WITHDRAWALS_SUSPENDED", "true", 0).Err()
	if err != nil {
		w.logger.Errorf("Failed to set Circuit Breaker in Redis! FATAL: %v", err)
	}

	// Log an explicit AUDIT_ALERT
	w.logger.Error("AUDIT_ALERT: System withdrawals have been suspended immediately due to reconciliation failure.")
}
