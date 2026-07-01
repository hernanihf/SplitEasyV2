package config

import (
	"log/slog"
	"os"
	"time"

	"github.com/redis/go-redis/v9"
)

// NewRedisClientFromEnv builds a Redis client from REDIS_URL, or reports
// false if it isn't set (or is invalid). Shared by anything that wants
// Redis-backed state (the scan rate limiter, refresh tokens) so they all
// talk to the same instance with the same tuned timeouts.
func NewRedisClientFromEnv() (*redis.Client, bool) {
	redisURL := os.Getenv("REDIS_URL")
	if redisURL == "" {
		return nil, false
	}

	opts, err := redis.ParseURL(redisURL)
	if err != nil {
		slog.Error("REDIS_URL is invalid", "error", err)
		return nil, false
	}
	// Default retries/timeouts make a Redis outage hang for several seconds
	// before giving up. Callers that fail open (the rate limiter) want that
	// to happen fast; callers that fail closed (auth) want a bounded wait
	// rather than none at all. Either way, tighter is better than the default.
	opts.DialTimeout = 2 * time.Second
	opts.MaxRetries = 1

	return redis.NewClient(opts), true
}
