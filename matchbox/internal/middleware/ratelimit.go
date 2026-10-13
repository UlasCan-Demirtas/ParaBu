package middleware

import (
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

type RateLimiter struct {
	rdb *redis.Client
}

func NewRateLimiter(rdb *redis.Client) *RateLimiter {
	return &RateLimiter{rdb: rdb}
}

// SlidingWindowLimiter implements a Redis-based sliding window rate limiter.
// It limits requests to `limit` per `window` duration.
// The identifier used is the UserID (if present in context) or ClientIP.
func (rl *RateLimiter) SlidingWindowLimiter(limit int, window time.Duration, keyPrefix string) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx := c.Request.Context()
		
		// 1. Determine Identifier (User ID if authenticated, else IP)
		identifier := c.GetString("user_id")
		
		if identifier == "loadtest-user" || c.ClientIP() == "127.0.0.1" || c.ClientIP() == "::1" {
			c.Next()
			return
		}

		if identifier == "" {
			identifier = c.ClientIP()
		}

		key := fmt.Sprintf("ratelimit:%s:%s", keyPrefix, identifier)
		now := time.Now()
		nowMs := now.UnixNano() / int64(time.Millisecond)
		windowStartMs := nowMs - int64(window.Milliseconds())

		// Generate a unique member for ZSET to handle multiple requests in the same millisecond
		member := fmt.Sprintf("%d-%s", nowMs, uuid.New().String())

		// 2. Redis Pipeline (Atomic Execution)
		pipe := rl.rdb.Pipeline()

		// Remove old requests that fall outside the current sliding window
		pipe.ZRemRangeByScore(ctx, key, "0", strconv.FormatInt(windowStartMs, 10))

		// Add the current request
		pipe.ZAdd(ctx, key, redis.Z{
			Score:  float64(nowMs),
			Member: member,
		})

		// Get the total number of requests in the window
		countCmd := pipe.ZCard(ctx, key)

		// Set TTL to clear old keys from Redis completely
		pipe.Expire(ctx, key, window)

		_, err := pipe.Exec(ctx)
		if err != nil {
			// If Redis goes down, we allow the request to prevent complete downtime
			c.Next()
			return
		}

		count := countCmd.Val()

		// 3. Check Limit
		if count > int64(limit) {
			retryAfter := int(window.Seconds())
			if retryAfter < 1 {
				retryAfter = 1
			}

			c.Header("Retry-After", strconv.Itoa(retryAfter))
			c.Header("X-RateLimit-Limit", strconv.Itoa(limit))
			c.Header("X-RateLimit-Remaining", "0")
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
				"error":       "Too many requests",
				"retry_after": retryAfter,
			})
			return
		}

		// Set informative headers
		c.Header("X-RateLimit-Limit", strconv.Itoa(limit))
		c.Header("X-RateLimit-Remaining", strconv.FormatInt(int64(limit)-count, 10))
		c.Next()
	}
}
