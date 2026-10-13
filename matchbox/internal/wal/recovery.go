package wal

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"matchbox/pkg/types"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

// RecoveryInterface exposes the methods required by RecoveryEngine to rebuild state
type RecoveryInterface interface {
	RestoreOrder(symbol string, order *types.Order) error
	ProcessLimitOrderSync(order *types.Order)
	ProcessMarketOrderSync(order *types.Order)
	CancelOrderSync(symbol string, orderID uuid.UUID)
}

type RecoveryEngine struct {
	walPath     string
	snapshotDir string
	logger      *zap.SugaredLogger
}

func NewRecoveryEngine(walPath, snapshotDir string, logger *zap.SugaredLogger) *RecoveryEngine {
	return &RecoveryEngine{
		walPath:     walPath,
		snapshotDir: snapshotDir,
		logger:      logger,
	}
}

// Recover rebuilds the entire matching engine state.
// It returns true if recovery was performed, false if starting from scratch.
func (re *RecoveryEngine) Recover(symbols []string, engine RecoveryInterface) (bool, error) {
	var maxSnapshotTs int64
	var recovered bool

	// 1. Load Snapshots for each symbol
	for _, sym := range symbols {
		ts, err := re.loadLatestSnapshot(sym, engine)
		if err != nil {
			if !os.IsNotExist(err) {
				re.logger.Warnf("Failed to load snapshot for %s: %v", sym, err)
			}
		} else {
			recovered = true
			if ts > maxSnapshotTs {
				maxSnapshotTs = ts
			}
		}
	}

	// 2. Replay WAL Deltas (Events that happened AFTER the latest snapshot)
	err := re.replayWAL(maxSnapshotTs, engine)
	if err != nil {
		if os.IsNotExist(err) {
			re.logger.Info("No WAL file found. Starting fresh.")
			return recovered, nil
		}
		return recovered, fmt.Errorf("WAL replay failed: %w", err)
	}

	return recovered, nil
}

func (re *RecoveryEngine) loadLatestSnapshot(symbol string, engine RecoveryInterface) (int64, error) {
	// Find latest snapshot file
	files, err := os.ReadDir(re.snapshotDir)
	if err != nil {
		return 0, err
	}

	var latestFile string
	var latestTs int64

	prefix := fmt.Sprintf("snapshot_%s_", symbol)
	for _, f := range files {
		if strings.HasPrefix(f.Name(), prefix) && strings.HasSuffix(f.Name(), ".bin") {
			var ts int64
			// Extract timestamp
			parts := strings.Split(f.Name(), "_")
			if len(parts) >= 3 {
				tsStr := strings.TrimSuffix(parts[len(parts)-1], ".bin")
				fmt.Sscanf(tsStr, "%d", &ts)
				if ts > latestTs {
					latestTs = ts
					latestFile = f.Name()
				}
			}
		}
	}

	if latestFile == "" {
		return 0, os.ErrNotExist
	}

	re.logger.Infof("Restoring snapshot %s for symbol %s...", latestFile, symbol)

	f, err := os.Open(filepath.Join(re.snapshotDir, latestFile))
	if err != nil {
		return 0, err
	}
	defer f.Close()

	var snap SnapshotData
	if err := json.NewDecoder(f).Decode(&snap); err != nil {
		return 0, err
	}

	for _, order := range snap.Orders {
		_ = engine.RestoreOrder(symbol, order)
	}

	re.logger.Infof("Restored %d resting orders for %s from snapshot.", len(snap.Orders), symbol)
	return snap.Timestamp, nil
}

func (re *RecoveryEngine) replayWAL(startTs int64, engine RecoveryInterface) error {
	re.logger.Infof("Replaying WAL from timestamp > %d", startTs)

	f, err := os.Open(re.walPath)
	if err != nil {
		return err
	}
	defer f.Close()

	reader := bufio.NewReader(f)
	decoder := json.NewDecoder(reader)

	var replayed int
	for {
		var entry WALEntry
		if err := decoder.Decode(&entry); err != nil {
			if err == io.EOF {
				break
			}
			re.logger.Warnf("WAL decoding error (corrupted entry?): %v", err)
			continue // Skip corrupted line
		}

		// Only process delta (events that happened after the snapshot)
		if entry.Timestamp <= startTs {
			continue
		}

		replayed++

		switch entry.Type {
		case EventOrderPlaced:
			id, _ := uuid.Parse(entry.OrderID)
			var tSide types.Side
			if entry.Side == "BUY" {
				tSide = types.SideBuy
			} else {
				tSide = types.SideSell
			}

			var tType types.OrderType
			if entry.OrderType == "LIMIT" {
				tType = types.OrderTypeLimit
			} else {
				tType = types.OrderTypeMarket
			}

			order := &types.Order{
				ID:           id,
				UserID:       entry.UserID,
				Symbol:       entry.Symbol,
				Side:         tSide,
				Type:         tType,
				Price:        entry.Price,
				Quantity:     entry.Quantity,
				RemainingQty: entry.Quantity,
				Status:       types.OrderStatusNew,
				CreatedAt:    time.Unix(0, entry.CreatedAt).UTC(),
			}

			// Replay matching synchronously
			if order.Type == types.OrderTypeLimit {
				engine.ProcessLimitOrderSync(order)
			} else {
				engine.ProcessMarketOrderSync(order)
			}

		case EventOrderCanceled:
			id, _ := uuid.Parse(entry.OrderID)
			engine.CancelOrderSync(entry.Symbol, id)
		}
	}

	re.logger.Infof("Replayed %d events from WAL.", replayed)
	return nil
}
