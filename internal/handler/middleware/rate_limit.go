package middleware

import (
	"log"
	"net/http"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
)

// ScanLimiter is satisfied by both ScanRateLimiter (in-memory) and
// RedisScanRateLimiter, so main.go can wire in whichever backend is
// available without the route registration caring which one it got.
type ScanLimiter interface {
	Limit(next http.Handler) http.Handler
}

// ScanRateLimiter is a simple in-memory, per-user token bucket. It caps how
// often a user can hit the receipt-scan endpoint, which is slow and billed by
// Anthropic, so a single account can't drain the budget with repeated uploads.
//
// Counters live only in this process's memory, so they reset on every
// restart/deploy — a user could in principle burn a fresh burst right after
// a deploy. This is used as the local-dev fallback when REDIS_URL isn't set;
// production should always have Redis configured (see NewScanRateLimiterFromEnv).
type ScanRateLimiter struct {
	mu       sync.Mutex
	buckets  map[uint]*scanBucket
	capacity float64 // max burst
	refill   float64 // tokens added per second
}

type scanBucket struct {
	tokens   float64
	last     time.Time
	lastSeen time.Time
}

// NewScanRateLimiter allows up to `burst` scans back-to-back and then refills at
// a sustained rate of `perHour` scans per hour.
func NewScanRateLimiter(perHour, burst int) *ScanRateLimiter {
	if perHour < 1 {
		perHour = 1
	}
	if burst < 1 {
		burst = 1
	}
	l := &ScanRateLimiter{
		buckets:  make(map[uint]*scanBucket),
		capacity: float64(burst),
		refill:   float64(perHour) / 3600.0,
	}
	go l.cleanupLoop()
	return l
}

// NewScanRateLimiterFromEnv reads SCAN_RATE_PER_HOUR (default 10) and
// SCAN_RATE_BURST (default 5). If REDIS_URL is set, counters are stored in
// Redis so they survive restarts/deploys and are shared across instances;
// otherwise it falls back to the in-memory limiter (fine for local dev, not
// for production — see the ScanRateLimiter doc comment).
func NewScanRateLimiterFromEnv() ScanLimiter {
	perHour := envInt("SCAN_RATE_PER_HOUR", 10)
	burst := envInt("SCAN_RATE_BURST", 5)

	redisURL := os.Getenv("REDIS_URL")
	if redisURL == "" {
		log.Println("REDIS_URL not set — scan rate limiter is in-memory and will reset on every deploy")
		return NewScanRateLimiter(perHour, burst)
	}

	opts, err := redis.ParseURL(redisURL)
	if err != nil {
		log.Printf("REDIS_URL is invalid (%v) — falling back to the in-memory rate limiter", err)
		return NewScanRateLimiter(perHour, burst)
	}
	// The Limit middleware fails open on any Redis error, but the default
	// client retries several times with backoff before giving up — during an
	// outage that would make every scan request hang for seconds first.
	// Tighten it so failing open actually happens quickly.
	opts.DialTimeout = 2 * time.Second
	opts.MaxRetries = 1

	return NewRedisScanRateLimiter(redis.NewClient(opts), perHour, burst)
}

func envInt(key string, fallback int) int {
	if v, err := strconv.Atoi(os.Getenv(key)); err == nil && v > 0 {
		return v
	}
	return fallback
}

func (l *ScanRateLimiter) allow(userID uint) (bool, time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := time.Now()
	b, ok := l.buckets[userID]
	if !ok {
		b = &scanBucket{tokens: l.capacity, last: now}
		l.buckets[userID] = b
	}

	b.tokens += now.Sub(b.last).Seconds() * l.refill
	if b.tokens > l.capacity {
		b.tokens = l.capacity
	}
	b.last = now
	b.lastSeen = now

	if b.tokens >= 1 {
		b.tokens--
		return true, 0
	}
	// Time until the bucket has one whole token again.
	wait := time.Duration((1 - b.tokens) / l.refill * float64(time.Second))
	return false, wait
}

// Limit is the HTTP middleware. It must run after JWTAuth so the user id is in
// the request context.
func (l *ScanRateLimiter) Limit(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		userID, ok := UserIDFromContext(r.Context())
		if !ok {
			http.Error(w, "invalid user id in token", http.StatusUnauthorized)
			return
		}
		if allowed, retry := l.allow(userID); !allowed {
			w.Header().Set("Retry-After", strconv.Itoa(int(retry.Seconds())+1))
			http.Error(w, "scan rate limit exceeded, please wait before scanning again", http.StatusTooManyRequests)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// cleanupLoop evicts buckets for users who haven't scanned in a while so the
// map doesn't grow without bound.
func (l *ScanRateLimiter) cleanupLoop() {
	ticker := time.NewTicker(30 * time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		l.mu.Lock()
		cutoff := time.Now().Add(-1 * time.Hour)
		for id, b := range l.buckets {
			if b.lastSeen.Before(cutoff) {
				delete(l.buckets, id)
			}
		}
		l.mu.Unlock()
	}
}
