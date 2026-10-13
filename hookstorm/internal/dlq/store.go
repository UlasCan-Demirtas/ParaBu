package dlq

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"
)

// Store handles inserting failed webhooks into a Dead Letter Queue table.
type Store struct {
	db     *pgxpool.Pool
	logger *zap.SugaredLogger
}

func NewStore(db *pgxpool.Pool, logger *zap.SugaredLogger) *Store {
	return &Store{
		db:     db,
		logger: logger,
	}
}

// Insert adds a failed event to the dead letter queue for manual review.
func (s *Store) Insert(ctx context.Context, endpoint string, payload []byte, failureReason string) error {
	_, err := s.db.Exec(ctx, `
		INSERT INTO dead_letter_queue (endpoint, payload, failure_reason)
		VALUES ($1, $2, $3)
	`, endpoint, string(payload), failureReason)
	
	if err != nil {
		s.logger.Errorf("Failed to insert into DLQ for %s: %v", endpoint, err)
		return err
	}
	
	s.logger.Warnf("Message for %s moved to Dead Letter Queue. Reason: %s", endpoint, failureReason)
	return nil
}
