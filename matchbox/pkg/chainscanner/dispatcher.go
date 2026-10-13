package chainscanner

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"
)

// DispatchPayload represents the validated deposit to be sent to Vault
type DispatchPayload struct {
	TxHash       string `json:"tx_hash"`
	LogIndex     uint   `json:"log_index"`
	UserAddress  string `json:"user_address"`
	AmountWei    string `json:"amount_wei"`
	IsERC20      bool   `json:"is_erc20"`
	TokenAddress string `json:"token_address"`
	Currency     string `json:"currency"`
}

// DepositDispatcher handles sending confirmed deposits to the Vault safely.
// Features: Idempotency, Exponential Backoff, Dead Letter Queue (DLQ).
type DepositDispatcher struct {
	db       *pgxpool.Pool
	logger   *zap.SugaredLogger
	vaultSvc VaultService
}

// VaultService interface abstracts the internal ledger call
type VaultService interface {
	CreditDeposit(ctx context.Context, idempotencyKey string, userID string, currency string, amount string) error
}

func NewDepositDispatcher(db *pgxpool.Pool, vaultSvc VaultService, logger *zap.SugaredLogger) *DepositDispatcher {
	return &DepositDispatcher{
		db:       db,
		logger:   logger,
		vaultSvc: vaultSvc,
	}
}

// Dispatch processes a confirmed deposit asynchronously
func (d *DepositDispatcher) Dispatch(deposit *PendingDeposit) {
	// 1. Create a globally unique Idempotency Key combining TxHash and LogIndex
	// This physically prevents double-spending even if the dispatcher restarts.
	idempotencyKey := fmt.Sprintf("%s:%d", deposit.TxHash.Hex(), deposit.LogIndex)

	payload := DispatchPayload{
		TxHash:       deposit.TxHash.Hex(),
		LogIndex:     deposit.LogIndex,
		UserAddress:  deposit.UserAddress,
		AmountWei:    deposit.AmountWei,
		IsERC20:      deposit.IsERC20,
		TokenAddress: deposit.TokenAddress,
	}

	// 2. Launch worker goroutine to process it with retries
	go d.processWithRetries(idempotencyKey, payload)
}

func (d *DepositDispatcher) processWithRetries(idempotencyKey string, payload DispatchPayload) {
	ctx := context.Background()
	maxRetries := 5
	backoffBase := time.Second

	// Map TokenAddress/Native to internal Currency Symbol
	currency := "ETH"
	if payload.IsERC20 {
		// In a real system, we look this up via a mapping cache
		currency = "ERC20_TOKEN"
	}

	// Wait, we need to map the user address to internal user ID.
	// For this architecture, we assume VaultService handles the mapping or we do it here.
	userID := payload.UserAddress // In production, resolve UserID from address_pool table

	for attempt := 1; attempt <= maxRetries; attempt++ {
		d.logger.Infof("🚀 Dispatching deposit %s to Vault (Attempt %d/%d)", idempotencyKey, attempt, maxRetries)

		// Calling the internal Ledger Vault service
		err := d.vaultSvc.CreditDeposit(ctx, idempotencyKey, userID, currency, payload.AmountWei)
		
		if err == nil {
			d.logger.Infof("✅ Successfully credited %s to Vault Ledger for Tx %s", payload.AmountWei, idempotencyKey)
			return
		}

		d.logger.Warnf("⚠️ Vault rejection for %s: %v", idempotencyKey, err)

		// Exponential Backoff (1s, 2s, 4s, 8s, 16s)
		if attempt < maxRetries {
			time.Sleep(backoffBase * time.Duration(1<<(attempt-1)))
		}
	}

	// 3. FAILED after 5 retries -> Send to Dead Letter Queue (DLQ)
	d.logger.Errorf("❌ FATAL: Deposit %s permanently failed. Routing to Dead Letter Queue (DLQ).", idempotencyKey)
	d.sendToDLQ(ctx, payload, "Max retries exceeded while calling VaultService")
}

func (d *DepositDispatcher) sendToDLQ(ctx context.Context, payload DispatchPayload, reason string) {
	payloadJSON, _ := json.Marshal(payload)

	// Ensure PostgreSQL DLQ table exists (or just insert assuming migration ran)
	_, err := d.db.Exec(ctx, `
		INSERT INTO dead_letter_queue (tx_hash, log_index, payload, error_reason)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (tx_hash, log_index) DO NOTHING
	`, payload.TxHash, payload.LogIndex, payloadJSON, reason)

	if err != nil {
		d.logger.Errorf("🚨 CRITICAL: Failed to write to DLQ for Tx %s: %v", payload.TxHash, err)
		// At this point, we'd fire an urgent PagerDuty alert to administrators
	} else {
		d.logger.Infof("📫 Deposit safely parked in Dead Letter Queue for manual review: %s", payload.TxHash)
	}
}
