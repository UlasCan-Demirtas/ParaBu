package chainscanner

import (
	"context"
	"math/big"
	"sync"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"
	"go.uber.org/zap"
)

// DepositStatus represents the lifecycle of a deposit
type DepositStatus string

const (
	StatusPendingConfirmation DepositStatus = "PENDING_CONFIRMATION"
	StatusConfirmed         DepositStatus = "CONFIRMED"
	StatusOrphaned          DepositStatus = "ORPHANED_REORG"
)

// PendingDeposit tracks a deposit that is waiting for block confirmations
type PendingDeposit struct {
	TxHash        common.Hash
	LogIndex      uint
	BlockNumber   uint64
	UserAddress   string
	AmountWei     string
	IsERC20       bool
	TokenAddress  string
	Status        DepositStatus
	Confirmations uint64
}

// ReorgHandler manages the state-machine for block finality and chain reorganizations.
type ReorgHandler struct {
	client           *ethclient.Client
	logger           *zap.SugaredLogger
	RequiredConfirms uint64
	dispatcher       *DepositDispatcher

	mu              sync.RWMutex
	// RecentBlocks maps BlockNumber -> BlockHash to detect forks
	RecentBlocks    map[uint64]common.Hash
	// Pending deposits awaiting finality
	PendingDeposits map[common.Hash]*PendingDeposit
}

func NewReorgHandler(client *ethclient.Client, reqConfirms uint64, dispatcher *DepositDispatcher, logger *zap.SugaredLogger) *ReorgHandler {
	return &ReorgHandler{
		client:           client,
		logger:           logger,
		RequiredConfirms: reqConfirms,
		dispatcher:       dispatcher,
		RecentBlocks:     make(map[uint64]common.Hash),
		PendingDeposits:  make(map[common.Hash]*PendingDeposit),
	}
}

// CheckReorg validates the parent hash. If a reorg is detected, it returns the common ancestor block.
func (r *ReorgHandler) CheckReorg(ctx context.Context, currentBlockNum uint64, currentParentHash common.Hash, currentHash common.Hash) (uint64, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()

	prevHash, exists := r.RecentBlocks[currentBlockNum-1]
	
	// If we don't have the previous block, we just record the current one and assume it's fine.
	if !exists {
		r.RecentBlocks[currentBlockNum] = currentHash
		return currentBlockNum, false
	}

	// Normal case: Chain is intact
	if prevHash == currentParentHash {
		r.RecentBlocks[currentBlockNum] = currentHash
		r.updateConfirmations(currentBlockNum)
		return currentBlockNum, false
	}

	// REORG DETECTED!
	r.logger.Warnf("🚨 CHAIN REORG DETECTED at Block %d! Expected Parent: %s, Actual Parent: %s", 
		currentBlockNum, prevHash.Hex(), currentParentHash.Hex())

	// Step back to find the common ancestor
	commonAncestor := r.findCommonAncestor(ctx, currentBlockNum-1, currentParentHash)
	
	r.logger.Warnf("🔄 Common ancestor found at Block %d. Unwinding state...", commonAncestor)
	r.unwindState(commonAncestor)

	// Record the new chain state from ancestor to current
	r.RecentBlocks[currentBlockNum] = currentHash
	return commonAncestor, true
}

// findCommonAncestor steps back via RPC to find where the fork started
func (r *ReorgHandler) findCommonAncestor(ctx context.Context, forkBlockNum uint64, forkBlockHash common.Hash) uint64 {
	currNum := forkBlockNum
	currHash := forkBlockHash

	for {
		recordedHash, exists := r.RecentBlocks[currNum]
		if exists && recordedHash == currHash {
			return currNum
		}

		// Fetch the block from the new branch to check its parent
		block, err := r.client.BlockByNumber(ctx, big.NewInt(int64(currNum)))
		if err != nil {
			r.logger.Errorf("RPC Error while finding ancestor for block %d: %v", currNum, err)
			return currNum // Fallback, just resume from here
		}

		currHash = block.ParentHash()
		currNum--

		if currNum == 0 {
			return 0
		}
	}
}

// unwindState rolls back blocks and cancels orphaned deposits
func (r *ReorgHandler) unwindState(ancestorBlock uint64) {
	// 1. Remove blocks higher than the common ancestor
	for num := range r.RecentBlocks {
		if num > ancestorBlock {
			delete(r.RecentBlocks, num)
		}
	}

	// 2. Unwind transactions (Cancel orphaned deposits)
	for txHash, deposit := range r.PendingDeposits {
		if deposit.BlockNumber > ancestorBlock {
			deposit.Status = StatusOrphaned
			r.logger.Errorf("❌ UNWIND DEPOSIT: Tx %s at Block %d is ORPHANED due to Reorg. Cancelling...", 
				txHash.Hex(), deposit.BlockNumber)
			
			// Remove from pending
			delete(r.PendingDeposits, txHash)
			
			// TODO: If we had incremented balance preemptively in DB (we shouldn't have),
			// we would dispatch a rollback message to Kafka/Ledger here.
		}
	}
}

// RegisterDeposit adds a new deposit to the state machine
func (r *ReorgHandler) RegisterDeposit(deposit *PendingDeposit) {
	r.mu.Lock()
	defer r.mu.Unlock()

	deposit.Status = StatusPendingConfirmation
	r.PendingDeposits[deposit.TxHash] = deposit
	r.logger.Infof("⏳ Deposit %s registered. Waiting for %d confirmations...", deposit.TxHash.Hex(), r.RequiredConfirms)
}

// updateConfirmations increments confirmation counts and finalizes deposits
func (r *ReorgHandler) updateConfirmations(currentBlockNum uint64) {
	for txHash, deposit := range r.PendingDeposits {
		// Calculate confirmations (Current - DepositBlock + 1)
		if currentBlockNum >= deposit.BlockNumber {
			deposit.Confirmations = (currentBlockNum - deposit.BlockNumber) + 1
		}

		if deposit.Confirmations >= r.RequiredConfirms {
			deposit.Status = StatusConfirmed
			r.logger.Infof("✅ DEPOSIT FINALIZED: Tx %s reached %d confirmations! Releasing funds.", 
				txHash.Hex(), deposit.Confirmations)
			
			// Dispatch to Vault
			if r.dispatcher != nil {
				r.dispatcher.Dispatch(deposit)
			}
			
			delete(r.PendingDeposits, txHash)
		}
	}

	// Clean up old cached blocks to prevent memory leaks (keep last 100)
	for num := range r.RecentBlocks {
		if num < currentBlockNum-100 {
			delete(r.RecentBlocks, num)
		}
	}
}
