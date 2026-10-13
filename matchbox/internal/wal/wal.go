package wal

import (
	"bufio"
	"encoding/json"
	"os"
	"sync"
	"time"

	"github.com/shopspring/decimal"
)

type EventType string

const (
	EventOrderPlaced   EventType = "ORDER_PLACED"
	EventOrderCanceled EventType = "ORDER_CANCELED"
	EventTradeExecuted EventType = "TRADE_EXECUTED"
)

// WALEntry represents a single transaction log record.
type WALEntry struct {
	Type      EventType       `json:"type"`
	Timestamp int64           `json:"timestamp"`
	OrderID   string          `json:"order_id,omitempty"`
	UserID    string          `json:"user_id,omitempty"`
	Symbol    string          `json:"symbol,omitempty"`
	Side      string          `json:"side,omitempty"`
	OrderType string          `json:"order_type,omitempty"`
	Price     decimal.Decimal `json:"price,omitempty"`
	Quantity  decimal.Decimal `json:"quantity,omitempty"`
	CreatedAt int64           `json:"created_at,omitempty"`
}

type WALWriter struct {
	mu         sync.Mutex
	file       *os.File
	bw         *bufio.Writer
	encoder    *json.Encoder
	syncTick   time.Duration
	stopCh     chan struct{}
	forceFsync bool
}

// NewWALWriter creates a new Write-Ahead Log.
// If forceFsync is true, every write will trigger an fsync (ultra-safe but slow).
// If false, it flushes the buffer and calls fsync every syncInterval (e.g. 10ms) for high throughput.
func NewWALWriter(path string, syncInterval time.Duration, forceFsync bool) (*WALWriter, error) {
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return nil, err
	}

	bw := bufio.NewWriterSize(f, 64*1024) // 64KB buffer for high throughput

	w := &WALWriter{
		file:       f,
		bw:         bw,
		encoder:    json.NewEncoder(bw),
		syncTick:   syncInterval,
		stopCh:     make(chan struct{}),
		forceFsync: forceFsync,
	}

	if !forceFsync && syncInterval > 0 {
		go w.backgroundFsync()
	}

	return w, nil
}

// WriteEntry thread-safely writes an entry to the WAL.
// This MUST complete successfully before the matching engine updates its memory.
func (w *WALWriter) WriteEntry(entry WALEntry) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	// Using JSON for prototype. For ultimate speed, this should be FlatBuffers or Protocol Buffers.
	if err := w.encoder.Encode(&entry); err != nil {
		return err
	}

	if w.forceFsync {
		if err := w.bw.Flush(); err != nil {
			return err
		}
		return w.file.Sync() // fsync to disk immediately
	}

	return nil
}

func (w *WALWriter) backgroundFsync() {
	ticker := time.NewTicker(w.syncTick)
	defer ticker.Stop()

	for {
		select {
		case <-w.stopCh:
			return
		case <-ticker.C:
			w.mu.Lock()
			if w.bw != nil && w.bw.Buffered() > 0 {
				_ = w.bw.Flush()
				_ = w.file.Sync() // Sync dirty pages to disk
			}
			w.mu.Unlock()
		}
	}
}

// Rotate renames the current WAL file and creates a new one, archiving the old history.
func (w *WALWriter) Rotate() error {
	w.mu.Lock()
	defer w.mu.Unlock()

	// Flush and close current
	if w.bw != nil {
		_ = w.bw.Flush()
	}
	if w.file != nil {
		_ = w.file.Sync()
		_ = w.file.Close()
	}

	// Rename current file to a timestamped backup
	oldPath := w.file.Name()
	archivedPath := oldPath + "." + time.Now().Format("20060102_150405") + ".bak"
	if err := os.Rename(oldPath, archivedPath); err != nil && !os.IsNotExist(err) {
		return err
	}

	// Open new file
	f, err := os.OpenFile(oldPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}

	w.file = f
	w.bw = bufio.NewWriterSize(f, 64*1024)
	w.encoder = json.NewEncoder(w.bw)

	return nil
}

func (w *WALWriter) Close() error {
	close(w.stopCh)
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.bw != nil {
		_ = w.bw.Flush()
	}
	if w.file != nil {
		_ = w.file.Sync()
		return w.file.Close()
	}
	return nil
}
