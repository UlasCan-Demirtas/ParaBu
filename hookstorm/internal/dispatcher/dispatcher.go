package dispatcher

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"time"

	"hookstorm/internal/circuit"
	"hookstorm/internal/dlq"
	"hookstorm/internal/ratelimit"
	"hookstorm/internal/retry"
	"hookstorm/internal/signing"
	"hookstorm/internal/worker"

	"github.com/nats-io/nats.go"
	"go.uber.org/zap"
)

type Dispatcher struct {
	nc        *nats.Conn
	logger    *zap.SugaredLogger
	pool      *worker.Pool
	limiter   *ratelimit.Limiter
	breaker   *circuit.Breaker
	dlq       *dlq.Store
	retry     *retry.Policy
	secret    string
	endpoint  string
	sub       *nats.Subscription
}

func NewDispatcher(nc *nats.Conn, logger *zap.SugaredLogger, pool *worker.Pool, limiter *ratelimit.Limiter, dlq *dlq.Store, secret, endpoint string) *Dispatcher {
	return &Dispatcher{
		nc:       nc,
		logger:   logger,
		pool:     pool,
		limiter:  limiter,
		breaker:  circuit.NewBreaker(endpoint, logger),
		dlq:      dlq,
		retry:    retry.NewPolicy(100*time.Millisecond, 2*time.Second, 5),
		secret:   secret,
		endpoint: endpoint,
	}
}

func (d *Dispatcher) Start(subject string) error {
	sub, err := d.nc.Subscribe(subject, func(msg *nats.Msg) {
		d.pool.Submit(func(ctx context.Context) {
			d.processMessage(ctx, msg.Data)
		})
	})
	if err != nil {
		return err
	}
	d.sub = sub
	d.logger.Infof("Dispatcher listening on %s", subject)
	return nil
}

func (d *Dispatcher) processMessage(ctx context.Context, payload []byte) {
	// 1. Rate Limit Check
	allowed, err := d.limiter.Allow(ctx, d.endpoint, 100, 10) // burst 100, rate 10/sec
	if err != nil || !allowed {
		d.logger.Warnf("Rate limit exceeded for %s", d.endpoint)
		d.dlq.Insert(ctx, d.endpoint, payload, "RATE_LIMIT_EXCEEDED")
		return
	}

	// 2. Circuit Breaker + Retry Loop
	var lastErr error
	for attempt := 1; attempt <= d.retry.MaxRetries; attempt++ {
		_, lastErr = d.breaker.Execute(func() (interface{}, error) {
			return nil, d.sendWebhook(ctx, payload)
		})

		if lastErr == nil {
			d.logger.Debugf("Successfully delivered webhook to %s on attempt %d", d.endpoint, attempt)
			return
		}

		delay := d.retry.NextDelay(attempt)
		d.logger.Warnf("Attempt %d failed: %v. Retrying in %v...", attempt, lastErr, delay)
		time.Sleep(delay)
	}

	// 3. Dead Letter Queue
	d.logger.Errorf("Failed to deliver webhook to %s after %d attempts", d.endpoint, d.retry.MaxRetries)
	d.dlq.Insert(ctx, d.endpoint, payload, fmt.Sprintf("EXHAUSTED_RETRIES: %v", lastErr))
}

func (d *Dispatcher) sendWebhook(ctx context.Context, payload []byte) error {
	req, err := http.NewRequestWithContext(ctx, "POST", d.endpoint, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	
	req.Header.Set("Content-Type", "application/json")
	
	// HMAC SHA256 Signature
	sig := signing.SignPayload(payload, d.secret)
	req.Header.Set("X-ParaBu-Signature", sig)

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	return nil
}

func (d *Dispatcher) Stop() {
	if d.sub != nil {
		d.sub.Unsubscribe()
	}
}
