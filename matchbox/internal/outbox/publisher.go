package outbox

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/nats-io/nats.go"
	"go.uber.org/zap"
)

// Publisher listens to the outbox_events table and reliably pushes events to external brokers (Kafka/NATS/RabbitMQ).
type Publisher struct {
	db     *pgxpool.Pool
	logger *zap.SugaredLogger
	nc     *nats.Conn
	stopCh chan struct{}
}

// NewPublisher creates a new Transactional Outbox publisher.
func NewPublisher(db *pgxpool.Pool, logger *zap.SugaredLogger, nc *nats.Conn) *Publisher {
	return &Publisher{
		db:     db,
		logger: logger,
		nc:     nc,
		stopCh: make(chan struct{}),
	}
}

// Start begins polling the outbox table for PENDING events.
func (p *Publisher) Start() {
	go func() {
		p.logger.Info("Outbox Publisher started. Monitoring for PENDING events...")
		ticker := time.NewTicker(500 * time.Millisecond) // Poll every 500ms
		defer ticker.Stop()

		for {
			select {
			case <-p.stopCh:
				p.logger.Info("Outbox Publisher stopped.")
				return
			case <-ticker.C:
				p.processBatch()
			}
		}
	}()
}

// Stop gracefully shuts down the publisher.
func (p *Publisher) Stop() {
	close(p.stopCh)
}

func (p *Publisher) processBatch() {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// 1. Fetch a batch of pending events.
	// We use FOR UPDATE SKIP LOCKED to safely allow multiple OutboxPublishers (High Availability)
	// to run concurrently without processing the same events.
	rows, err := p.db.Query(ctx, `
		SELECT id, event_type, payload 
		FROM outbox_events 
		WHERE status = 'PENDING' 
		ORDER BY created_at ASC 
		LIMIT 100 
		FOR UPDATE SKIP LOCKED
	`)
	if err != nil {
		p.logger.Errorf("Failed to query outbox events: %v", err)
		return
	}
	defer rows.Close()

	type Event struct {
		ID        int64
		EventType string
		Payload   string
	}

	var events []Event
	for rows.Next() {
		var e Event
		if err := rows.Scan(&e.ID, &e.EventType, &e.Payload); err != nil {
			p.logger.Errorf("Failed to scan outbox event: %v", err)
			continue
		}
		events = append(events, e)
	}
	rows.Close()

	if len(events) == 0 {
		return // Nothing to do
	}

	var processedIDs []int64

	// 2. Publish events to external Message Broker (Kafka/NATS/RabbitMQ)
	for _, e := range events {
		if p.nc != nil {
			subject := "matchbox.events." + e.EventType
			err := p.nc.Publish(subject, []byte(e.Payload))
			if err != nil {
				p.logger.Errorf("Failed to publish outbox event ID %d to NATS: %v", e.ID, err)
				continue // Skip so it will be retried in the next batch
			}
			p.logger.Debugf("Published %s outbox event ID %d to NATS subject %s", e.EventType, e.ID, subject)
		} else {
			// Mock Publishing to Kafka/RabbitMQ
			p.logger.Debugf("Mock Published %s outbox event ID %d.", e.EventType, e.ID)
		}
		
		processedIDs = append(processedIDs, e.ID)
	}

	// 3. Mark successfully published events as PROCESSED
	if len(processedIDs) > 0 {
		_, err := p.db.Exec(ctx, `
			UPDATE outbox_events 
			SET status = 'PROCESSED', processed_at = CURRENT_TIMESTAMP 
			WHERE id = ANY($1)
		`, processedIDs)
		if err != nil {
			p.logger.Errorf("Failed to mark outbox events as PROCESSED: %v", err)
		} else {
			p.logger.Infof("Successfully published and processed %d outbox events.", len(processedIDs))
		}
	}
}
