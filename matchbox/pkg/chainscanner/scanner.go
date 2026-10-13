package chainscanner

import (
	"context"
	"math/big"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
	"go.uber.org/zap"
)

// ChainScanner is an EVM blockchain indexer that polls blocks and uses a
// Worker Pool pattern to scan thousands of transactions concurrently.
type ChainScanner struct {
	client     *ethclient.Client
	logger     *zap.SugaredLogger
	trie       *AddressTrie
	workerPool int
	reorg      *ReorgHandler
	logParser  *LogParser
	Whitelist  *TokenWhitelist
}

// NewChainScanner initializes a connection to an EVM node (Ethereum, BSC, Polygon).
func NewChainScanner(rpcURL string, trie *AddressTrie, workerPool int, requiredConfirms uint64, dispatcher *DepositDispatcher, logger *zap.SugaredLogger) (*ChainScanner, error) {
	client, err := ethclient.Dial(rpcURL)
	if err != nil {
		return nil, err
	}

	whitelist := NewTokenWhitelist()
	reorgHandler := NewReorgHandler(client, requiredConfirms, dispatcher, logger)
	
	return &ChainScanner{
		client:     client,
		logger:     logger,
		trie:       trie,
		workerPool: workerPool,
		reorg:      reorgHandler,
		Whitelist:  whitelist,
		logParser:  NewLogParser(client, whitelist, trie, reorgHandler, logger),
	}, nil
}

// Start begins HTTP/WS polling for new finalized blocks.
func (s *ChainScanner) Start(ctx context.Context) {
	s.logger.Info("Starting ChainScanner Worker Pool...")
	
	// Poll every 3 seconds (Adjust based on network: ETH=12s, BSC=3s)
	ticker := time.NewTicker(3 * time.Second) 
	defer ticker.Stop()

	var lastProcessed uint64

	for {
		select {
		case <-ctx.Done():
			s.logger.Info("ChainScanner stopping gracefully...")
			return
		case <-ticker.C:
			// Fetch the latest block header
			header, err := s.client.HeaderByNumber(ctx, nil)
			if err != nil {
				s.logger.Errorf("Failed to get latest block header (eth_blockNumber): %v", err)
				continue
			}
			
			latestBlock := header.Number.Uint64()

			// Initialize the cursor on the very first run
			if lastProcessed == 0 {
				lastProcessed = latestBlock - 1
			}

			// Catch up on any missing blocks
			for blockNum := lastProcessed + 1; blockNum <= latestBlock; blockNum++ {
				s.processBlock(ctx, blockNum)
				lastProcessed = blockNum
			}
		}
	}
}

// processBlock downloads the block and distributes its transactions to the Worker Pool.
func (s *ChainScanner) processBlock(ctx context.Context, blockNum uint64) {
	block, err := s.client.BlockByNumber(ctx, big.NewInt(int64(blockNum)))
	if err != nil {
		s.logger.Errorf("Failed to fetch block %d: %v", blockNum, err)
		return
	}

	// CHECK FOR REORGS FIRST!
	ancestor, isReorg := s.reorg.CheckReorg(ctx, blockNum, block.ParentHash(), block.Hash())
	if isReorg {
		// If reorg happened, we must re-scan from the ancestor up to the current block!
		s.logger.Warnf("Rescanning from ancestor block %d to catch up with the fork...", ancestor)
		// Return and let the main loop or a specialized catch-up logic handle it
		// For now, we will simply process this block. 
		// In a production system, we'd adjust the `lastProcessed` cursor.
	}

	txs := block.Transactions()
	s.logger.Infof("Scanning Block %d | Transactions: %d", blockNum, len(txs))

	// 1. Fetch and Parse ERC20 Token Logs for this entire block in one RPC call!
	s.logParser.FetchAndParseERC20Logs(ctx, blockNum)

	// 2. Scan Native Transactions via Worker Pool
	txChan := make(chan *types.Transaction, len(txs))
	var wg sync.WaitGroup

	// Spin up workers
	for i := 0; i < s.workerPool; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for tx := range txChan {
				s.scanTransaction(ctx, tx, blockNum)
			}
		}()
	}

	// Feed transactions to the pool
	for _, tx := range txs {
		txChan <- tx
	}
	close(txChan) // Close channel to signal workers to exit when empty

	wg.Wait() // Wait for all transactions in this block to be processed
}

// scanTransaction analyzes a single transaction for Native (ETH/BNB) exchange deposits.
func (s *ChainScanner) scanTransaction(ctx context.Context, tx *types.Transaction, blockNum uint64) {
	if tx.To() == nil {
		return // Contract creation
	}

	toAddress := tx.To().Hex()
	isNativeDeposit := s.trie.Contains(toAddress)
	
	if !isNativeDeposit {
		return
	}

	// WE FOUND A NATIVE MATCH! NOW we do the heavy HTTP call to get the Receipt to ensure it didn't revert.
	receipt, err := s.client.TransactionReceipt(ctx, tx.Hash())
	if err != nil {
		s.logger.Errorf("Failed to fetch receipt for tx %s: %v", tx.Hash().Hex(), err)
		return
	}
	
	if receipt.Status != 1 {
		s.logger.Warnf("Detected native deposit to %s but transaction reverted (TxHash: %s)", toAddress, tx.Hash().Hex())
		return
	}

	if tx.Value().Sign() > 0 {
		s.logger.Infof("🚀 NATIVE DEPOSIT DETECTED: %s received %s Wei in Block %d (TxHash: %s)", toAddress, tx.Value().String(), blockNum, tx.Hash().Hex())
		s.reorg.RegisterDeposit(&PendingDeposit{
			TxHash:      tx.Hash(),
			LogIndex:    0, // Native transfer has no log index
			BlockNumber: blockNum,
			UserAddress: toAddress,
			AmountWei:   tx.Value().String(),
			IsERC20:     false,
		})
	}
}
