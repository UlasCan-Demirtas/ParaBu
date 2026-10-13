package retry

import (
	"math/rand"
	"time"
)

// Policy defines a retry strategy.
type Policy struct {
	BaseDelay  time.Duration
	MaxDelay   time.Duration
	MaxRetries int
}

// NewPolicy creates a new Exponential Backoff with Full Jitter retry policy.
func NewPolicy(baseDelay, maxDelay time.Duration, maxRetries int) *Policy {
	return &Policy{
		BaseDelay:  baseDelay,
		MaxDelay:   maxDelay,
		MaxRetries: maxRetries,
	}
}

// NextDelay calculates the delay for the next attempt using Exponential Backoff + Full Jitter.
func (p *Policy) NextDelay(attempt int) time.Duration {
	if attempt <= 0 {
		return 0
	}
	
	// Exponential backoff: base * 2^(attempt-1)
	backoff := float64(p.BaseDelay) * float64(int(1)<<(attempt-1))
	
	// Cap the backoff
	if backoff > float64(p.MaxDelay) {
		backoff = float64(p.MaxDelay)
	}

	// Full Jitter: rand(0, backoff)
	jitter := rand.Float64() * backoff

	return time.Duration(jitter)
}
