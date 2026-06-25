package middleware

import (
	"net/http"
	"os"
	"strconv"
	"sync"
	"time"
)

// ScanRateLimiter is a simple in-memory, per-user token bucket. It caps how
// often a user can hit the receipt-scan endpoint, which is slow and billed by
// Anthropic, so a single account can't drain the budget with repeated uploads.
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
// SCAN_RATE_BURST (default 5).
func NewScanRateLimiterFromEnv() *ScanRateLimiter {
	return NewScanRateLimiter(envInt("SCAN_RATE_PER_HOUR", 10), envInt("SCAN_RATE_BURST", 5))
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
