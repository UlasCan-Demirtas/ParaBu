package chainscanner

import (
	"context"
	"math/big"
	"strings"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
	"go.uber.org/zap"
)

// ERC20TransferSignature is the keccak256 hash of "Transfer(address,address,uint256)"
// Result: 0xddf252ad1be2c89b69c2b068fc378daa952ba7f163c4a11628f55a4df523b3ef
var ERC20TransferSignature = crypto.Keccak256Hash([]byte("Transfer(address,address,uint256)"))

// TokenWhitelist manages the ERC20 contracts that the exchange supports
type TokenWhitelist struct {
	whitelist map[common.Address]string // ContractAddress -> TokenSymbol
}

func NewTokenWhitelist() *TokenWhitelist {
	return &TokenWhitelist{
		whitelist: make(map[common.Address]string),
	}
}

func (w *TokenWhitelist) AddToken(contractAddress, symbol string) {
	addr := common.HexToAddress(contractAddress)
	w.whitelist[addr] = strings.ToUpper(symbol)
}

func (w *TokenWhitelist) GetSymbol(contractAddress common.Address) (string, bool) {
	symbol, exists := w.whitelist[contractAddress]
	return symbol, exists
}

// LogParser handles the parsing of eth_getLogs for ERC20 transfers
type LogParser struct {
	client    *ethclient.Client
	logger    *zap.SugaredLogger
	whitelist *TokenWhitelist
	trie      *AddressTrie
	reorg     *ReorgHandler
}

func NewLogParser(client *ethclient.Client, whitelist *TokenWhitelist, trie *AddressTrie, reorg *ReorgHandler, logger *zap.SugaredLogger) *LogParser {
	return &LogParser{
		client:    client,
		logger:    logger,
		whitelist: whitelist,
		trie:      trie,
		reorg:     reorg,
	}
}

// FetchAndParseERC20Logs uses eth_getLogs to fetch all Transfer events in a specific block
func (p *LogParser) FetchAndParseERC20Logs(ctx context.Context, blockNum uint64) {
	// 1. Create the RPC Filter Query for the specific block
	// We only want logs matching the ERC20 Transfer topic signature
	query := ethereum.FilterQuery{
		FromBlock: big.NewInt(int64(blockNum)),
		ToBlock:   big.NewInt(int64(blockNum)),
		Topics: [][]common.Hash{
			{ERC20TransferSignature},
		},
	}

	// 2. Fetch logs (eth_getLogs)
	logs, err := p.client.FilterLogs(ctx, query)
	if err != nil {
		p.logger.Errorf("Failed to fetch ERC20 logs for block %d: %v", blockNum, err)
		return
	}

	for _, vLog := range logs {
		// 3. Verify Contract Address is in our Whitelist
		symbol, isWhitelisted := p.whitelist.GetSymbol(vLog.Address)
		if !isWhitelisted {
			continue // We don't support this token
		}

		// ERC20 Transfer must have exactly 3 topics: Signature, From, To
		if len(vLog.Topics) != 3 {
			continue
		}

		// 4. Extract 'To' address from Topics[2]
		// Topics in EVM are left-padded with zeroes to 32 bytes.
		// Ethereum addresses are the last 20 bytes.
		toAddressHash := vLog.Topics[2]
		toAddress := common.BytesToAddress(toAddressHash.Bytes()[12:])

		// 5. Check if the recipient is one of our exchange users
		if p.trie.Contains(toAddress.Hex()) {
			// 6. Extract the transfer amount from the Log Data
			// Data contains the un-indexed uint256 amount
			amount := new(big.Int)
			amount.SetBytes(vLog.Data)

			p.logger.Infof("🪙 %s ERC20 DEPOSIT DETECTED: %s received %s (Wei) from contract %s (Tx: %s)",
				symbol, toAddress.Hex(), amount.String(), vLog.Address.Hex(), vLog.TxHash.Hex())

			// 7. Push to State Machine (PendingDeposits)
			p.reorg.RegisterDeposit(&PendingDeposit{
				TxHash:       vLog.TxHash,
				LogIndex:     uint(vLog.Index),
				BlockNumber:  blockNum,
				UserAddress:  toAddress.Hex(),
				AmountWei:    amount.String(),
				IsERC20:      true,
				TokenAddress: vLog.Address.Hex(),
			})
		}
	}
}
