package circuit

import (
	"time"

	"github.com/sony/gobreaker"
	"go.uber.org/zap"
)

// Breaker wraps the gobreaker.CircuitBreaker
type Breaker struct {
	cb     *gobreaker.CircuitBreaker
	logger *zap.SugaredLogger
}

// NewBreaker initializes a circuit breaker for a specific webhook endpoint.
func NewBreaker(name string, logger *zap.SugaredLogger) *Breaker {
	settings := gobreaker.Settings{
		Name:        name,
		MaxRequests: 5,                // Max requests allowed to pass through when half-open
		Interval:    10 * time.Second, // Cyclic period in closed state to clear counts
		Timeout:     30 * time.Second, // Time in open state before transitioning to half-open
		ReadyToTrip: func(counts gobreaker.Counts) bool {
			// Trip if there are more than 10 consecutive failures
			return counts.ConsecutiveFailures > 10
		},
		OnStateChange: func(name string, from gobreaker.State, to gobreaker.State) {
			logger.Warnf("CircuitBreaker '%s' state changed from %s to %s", name, from.String(), to.String())
		},
	}

	return &Breaker{
		cb:     gobreaker.NewCircuitBreaker(settings),
		logger: logger,
	}
}

// Execute runs the given function within the circuit breaker context.
func (b *Breaker) Execute(req func() (interface{}, error)) (interface{}, error) {
	return b.cb.Execute(req)
}
