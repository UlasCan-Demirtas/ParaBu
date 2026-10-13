package ratelimit

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

var tokenBucketScript = redis.NewScript(`
local rate_limit_key = KEYS[1]
local capacity = tonumber(ARGV[1])
local refill_rate = tonumber(ARGV[2])
local now = tonumber(ARGV[3])
local requested = tonumber(ARGV[4])

local bucket = redis.call("HMGET", rate_limit_key, "tokens", "last_refill")
local tokens = tonumber(bucket[1])
local last_refill = tonumber(bucket[2])

if tokens == nil then
    tokens = capacity
    last_refill = now
else
    local time_passed = now - last_refill
    local refill_amount = math.floor(time_passed * (refill_rate / 1000))
    if refill_amount > 0 then
        tokens = math.min(capacity, tokens + refill_amount)
        last_refill = now
    end
end

if tokens >= requested then
    tokens = tokens - requested
    redis.call("HMSET", rate_limit_key, "tokens", tokens, "last_refill", last_refill)
    redis.call("PEXPIRE", rate_limit_key, 60000) -- expire in 60 seconds
    return 1
else
    redis.call("HMSET", rate_limit_key, "tokens", tokens, "last_refill", last_refill)
    redis.call("PEXPIRE", rate_limit_key, 60000)
    return 0
end
`)

type Limiter struct {
	rdb    *redis.Client
	logger *zap.SugaredLogger
}

func NewLimiter(rdb *redis.Client, logger *zap.SugaredLogger) *Limiter {
	return &Limiter{
		rdb:    rdb,
		logger: logger,
	}
}

// Allow checks if the client/endpoint has enough tokens.
// capacity: max burst size.
// refillRate: tokens per second.
func (l *Limiter) Allow(ctx context.Context, key string, capacity, refillRate int) (bool, error) {
	now := time.Now().UnixMilli()
	
	// Pass arguments: capacity, refillRate (converted to refill per second), now, requested tokens (1)
	res, err := tokenBucketScript.Run(ctx, l.rdb, []string{key}, capacity, refillRate, now, 1).Result()
	if err != nil {
		l.logger.Errorf("Rate limiter error for key %s: %v", key, err)
		return false, err
	}

	allowed := res.(int64) == 1
	return allowed, nil
}
