package wal

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"matchbox/pkg/types"
)

type SnapshotData struct {
	Timestamp int64          `json:"timestamp"`
	Symbol    string         `json:"symbol"`
	Orders    []*types.Order `json:"orders"`
}

type SnapshotManager struct {
	mu           sync.Mutex
	walWriter    *WALWriter
	snapshotDir  string
	triggerCount int64 // e.g., 100000 events
	eventCounts  map[string]int64
}

// NewSnapshotManager creates a snapshot manager that saves state to snapshotDir
func NewSnapshotManager(w *WALWriter, dir string, triggerCount int64) *SnapshotManager {
	os.MkdirAll(dir, 0755)
	return &SnapshotManager{
		walWriter:    w,
		snapshotDir:  dir,
		triggerCount: triggerCount,
		eventCounts:  make(map[string]int64),
	}
}

// CheckAndSnapshot checks if the threshold is met, and if so, writes a snapshot and rotates the WAL.
func (sm *SnapshotManager) CheckAndSnapshot(symbol string, exportFunc func() []*types.Order) error {
	sm.mu.Lock()
	sm.eventCounts[symbol]++
	shouldSnapshot := sm.eventCounts[symbol] >= sm.triggerCount
	if shouldSnapshot {
		sm.eventCounts[symbol] = 0 // Reset counter
	}
	sm.mu.Unlock()

	if !shouldSnapshot {
		return nil
	}

	// Retrieve orders by calling the provided closure.
	// This should be executed in the same thread as the OrderBook actor to prevent races!
	orders := exportFunc()

	ts := time.Now().UnixNano()
	filename := filepath.Join(sm.snapshotDir, fmt.Sprintf("snapshot_%s_%d.bin", symbol, ts))

	snap := SnapshotData{
		Timestamp: ts,
		Symbol:    symbol,
		Orders:    orders,
	}

	f, err := os.OpenFile(filename, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		return err
	}
	defer f.Close()

	if err := json.NewEncoder(f).Encode(&snap); err != nil {
		return err
	}
	_ = f.Sync()

	// Once snapshot is written, we can rotate the WAL safely.
	return sm.walWriter.Rotate()
}
