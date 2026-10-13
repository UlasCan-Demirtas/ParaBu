package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"matchbox/internal/marketdata"
)

type KlineRepo struct {
	db *pgxpool.Pool
}

func NewKlineRepo(db *pgxpool.Pool) *KlineRepo {
	return &KlineRepo{
		db: db,
	}
}

// BatchInsertKlines inserts a large slice of klines into the TimescaleDB hypertable using PostgreSQL's COPY protocol.
// This is significantly faster than using standard INSERT statements for bulk data.
func (r *KlineRepo) BatchInsertKlines(ctx context.Context, symbol, interval string, klines []marketdata.Kline) error {
	// Prepare the data for pgx.CopyFromRows
	rows := make([][]interface{}, 0, len(klines))
	for _, k := range klines {
		// Convert Unix timestamp (seconds) to time.Time
		ts := time.Unix(k.OpenTime, 0).UTC()
		rows = append(rows, []interface{}{
			ts,
			symbol,
			interval,
			k.Open,
			k.High,
			k.Low,
			k.Close,
			k.Volume,
		})
	}

	tx, err := r.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	// Perform the COPY operation
	_, err = tx.CopyFrom(
		ctx,
		pgx.Identifier{"candlesticks"},
		[]string{"ts", "symbol", "interval", "open", "high", "low", "close", "volume"},
		pgx.CopyFromRows(rows),
	)
	if err != nil {
		return fmt.Errorf("failed to copy klines into candlesticks table: %w", err)
	}

	return tx.Commit(ctx)
}

// GetKlines reads chunks of historical candlesticks for a specific symbol, interval, and date range.
func (r *KlineRepo) GetKlines(ctx context.Context, symbol, interval string, startTime, endTime time.Time, limit int) ([]marketdata.Kline, error) {
	// Query leveraging TimescaleDB's chunked architecture and our composite index
	query := `
		SELECT 
			EXTRACT(EPOCH FROM ts)::bigint AS open_time,
			open, high, low, close, volume
		FROM candlesticks
		WHERE symbol = $1 
		  AND interval = $2 
		  AND ts >= $3 
		  AND ts <= $4
		ORDER BY ts ASC
		LIMIT $5
	`

	rows, err := r.db.Query(ctx, query, symbol, interval, startTime, endTime, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to query klines: %w", err)
	}
	defer rows.Close()

	var klines []marketdata.Kline
	for rows.Next() {
		var k marketdata.Kline
		err := rows.Scan(
			&k.OpenTime,
			&k.Open,
			&k.High,
			&k.Low,
			&k.Close,
			&k.Volume,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan kline row: %w", err)
		}
		klines = append(klines, k)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows iteration error: %w", err)
	}

	return klines, nil
}
