package middleware

import (
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/go-redis/redis_rate/v10"
	"github.com/redis/go-redis/v9"
)

// RedisScanRateLimiter enforces the same per-user token-bucket policy as
// ScanRateLimiter, but keeps the counters in Redis (via the GCRA algorithm
// in redis_rate) instead of process memory, so they survive restarts/deploys
// and are shared across every instance of the API.
type RedisScanRateLimiter struct {
	limiter *redis_rate.Limiter
	limit   redis_rate.Limit
}

// NewRedisScanRateLimiter allows up to `burst` scans back-to-back and then
// refills at a sustained rate of `perHour` scans per hour, using rdb to store
// the per-user counters.
func NewRedisScanRateLimiter(rdb *redis.Client, perHour, burst int) *RedisScanRateLimiter {
	if perHour < 1 {
		perHour = 1
	}
	if burst < 1 {
		burst = 1
	}
	return &RedisScanRateLimiter{
		limiter: redis_rate.NewLimiter(rdb),
		limit:   redis_rate.Limit{Rate: perHour, Period: time.Hour, Burst: burst},
	}
}

// Limit is the HTTP middleware. It must run after JWTAuth so the user id is in
// the request context.
func (l *RedisScanRateLimiter) Limit(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		userID, ok := UserIDFromContext(r.Context())
		if !ok {
			http.Error(w, "invalid user id in token", http.StatusUnauthorized)
			return
		}

		res, err := l.limiter.Allow(r.Context(), "scan:"+strconv.FormatUint(uint64(userID), 10), l.limit)
		if err != nil {
			// Redis being unreachable shouldn't take the whole (paid, but
			// otherwise healthy) scan feature down — fail open and log so
			// the outage is visible without blocking every user's requests.
			log.Printf("rate limiter: redis error, allowing request: %v", err)
			next.ServeHTTP(w, r)
			return
		}

		if res.Allowed < 1 {
			w.Header().Set("Retry-After", strconv.Itoa(int(res.RetryAfter.Seconds())+1))
			http.Error(w, "scan rate limit exceeded, please wait before scanning again", http.StatusTooManyRequests)
			return
		}
		next.ServeHTTP(w, r)
	})
}
