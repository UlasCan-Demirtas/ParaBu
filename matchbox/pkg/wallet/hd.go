package wallet

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/tyler-smith/go-bip32"
	"go.uber.org/zap"
)

// AddressManager orchestrates Hierarchical Deterministic (HD) Wallet operations.
// It pre-generates addresses and atomically allocates them to new users.
type AddressManager struct {
	db     *pgxpool.Pool
	logger *zap.SugaredLogger
	xpub   *bip32.Key
	mu     sync.Mutex
}

// NewAddressManager initializes the HD wallet using the provided xpub.
// IMPORTANT: The xpub must be the key derived at m/44'/60'/0'.
// The server will then derive the external chain (0) and index (m/44'/60'/0'/0/index).
// This guarantees the server NEVER sees the private keys!
func NewAddressManager(db *pgxpool.Pool, xpubStr string, logger *zap.SugaredLogger) (*AddressManager, error) {
	parsedXpub, err := bip32.B58Deserialize(xpubStr)
	if err != nil {
		return nil, fmt.Errorf("failed to parse xpub: %v", err)
	}

	if parsedXpub.IsPrivate {
		return nil, errors.New("FATAL: Private key passed to AddressManager! Must use xpub only for security")
	}

	return &AddressManager{
		db:     db,
		logger: logger,
		xpub:   parsedXpub,
	}, nil
}

// DeriveEthereumAddress generates an Ethereum address dynamically without knowing the private key.
func (m *AddressManager) DeriveEthereumAddress(index uint32) (string, error) {
	// 1. Derive External Chain: m/44'/60'/0'/0
	externalChain, err := m.xpub.NewChildKey(0) // 0 for external (receiving) addresses
	if err != nil {
		return "", err
	}

	// 2. Derive Index: m/44'/60'/0'/0/index
	childKey, err := externalChain.NewChildKey(index)
	if err != nil {
		return "", err
	}

	// 3. Convert 33-byte compressed public key to secp256k1 *ecdsa.PublicKey
	pubKey, err := crypto.DecompressPubkey(childKey.PublicKey().Key)
	if err != nil {
		return "", err
	}

	// 4. Generate Ethereum address (last 20 bytes of Keccak256 hash of the public key)
	address := crypto.PubkeyToAddress(*pubKey).Hex()

	return address, nil
}

// PreGeneratePool pre-generates N unused addresses in the database asynchronously.
func (m *AddressManager) PreGeneratePool(ctx context.Context, currency string, batchSize int) {
	m.logger.Infof("Pre-generating %d new HD addresses for %s pool...", batchSize, currency)

	// Find the highest derivation index currently in the database
	var maxIndex int64
	err := m.db.QueryRow(ctx, "SELECT COALESCE(MAX(derivation_index), -1) FROM address_pool WHERE currency = $1", currency).Scan(&maxIndex)
	if err != nil {
		m.logger.Errorf("Failed to query max derivation index: %v", err)
		return
	}

	startIndex := uint32(maxIndex + 1)
	for i := uint32(0); i < uint32(batchSize); i++ {
		index := startIndex + i
		addr, err := m.DeriveEthereumAddress(index)
		if err != nil {
			m.logger.Errorf("Failed to derive address at index %d: %v", index, err)
			continue
		}

		_, err = m.db.Exec(ctx, `
			INSERT INTO address_pool (currency, address, derivation_index, is_allocated) 
			VALUES ($1, $2, $3, false)
			ON CONFLICT (currency, derivation_index) DO NOTHING
		`, currency, addr, index)

		if err != nil {
			m.logger.Errorf("Failed to insert pre-generated address to pool: %v", err)
		}
	}
	m.logger.Infof("Successfully expanded address pool for %s.", currency)
}

// AllocateAddress atomically assigns a pre-generated, unused address to a user.
func (m *AddressManager) AllocateAddress(ctx context.Context, currency, userID string) (string, error) {
	// Atomic UPDATE via PostgreSQL RETURNING clause to prevent race conditions (Skip Locked)
	var allocatedAddress string
	err := m.db.QueryRow(ctx, `
		UPDATE address_pool 
		SET is_allocated = true, allocated_to = $1, allocated_at = $2
		WHERE id = (
			SELECT id FROM address_pool 
			WHERE currency = $3 AND is_allocated = false 
			ORDER BY derivation_index ASC 
			FOR UPDATE SKIP LOCKED 
			LIMIT 1
		)
		RETURNING address
	`, userID, time.Now(), currency).Scan(&allocatedAddress)

	if err != nil {
		// If pool is empty, trigger an emergency pre-generation and retry (or return error)
		m.logger.Warnf("Address pool empty for %s! Triggering emergency generation.", currency)
		go m.PreGeneratePool(context.Background(), currency, 100)
		return "", errors.New("address pool depleted, generating new addresses, try again shortly")
	}

	m.logger.Infof("✅ Atomically allocated deposit address %s to user %s", allocatedAddress, userID)
	return allocatedAddress, nil
}
