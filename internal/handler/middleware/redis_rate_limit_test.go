package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

func newTestRedisLimiter(t *testing.T, perHour, burst int) *RedisScanRateLimiter {
	t.Helper()
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("failed to start miniredis: %v", err)
	}
	t.Cleanup(mr.Close)

	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })

	return NewRedisScanRateLimiter(rdb, perHour, burst)
}

func requestAs(userID float64) *http.Request {
	r := httptest.NewRequest(http.MethodPost, "/receipts/scan", nil)
	return r.WithContext(context.WithValue(r.Context(), UserIDKey, userID))
}

func TestRedisScanRateLimiter_BurstThenDeny(t *testing.T) {
	l := newTestRedisLimiter(t, 10, 2) // burst of 2
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) })
	handler := l.Limit(next)

	for i := 0; i < 2; i++ {
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, requestAs(1))
		if rec.Code != http.StatusOK {
			t.Fatalf("request %d: expected 200 (within burst), got %d", i+1, rec.Code)
		}
	}

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, requestAs(1))
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429 once the burst is exhausted, got %d", rec.Code)
	}
	if rec.Header().Get("Retry-After") == "" {
		t.Error("expected a Retry-After header on the 429 response")
	}

	// A different user has their own counter.
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, requestAs(2))
	if rec.Code != http.StatusOK {
		t.Fatalf("a different user should not be affected, got %d", rec.Code)
	}
}

func TestRedisScanRateLimiter_RejectsMissingUserID(t *testing.T) {
	l := newTestRedisLimiter(t, 10, 5)
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) })
	handler := l.Limit(next)

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/receipts/scan", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 without a user id in context, got %d", rec.Code)
	}
}

func TestRedisScanRateLimiter_FailsOpenWhenRedisIsDown(t *testing.T) {
	// Nothing is listening here, so every call errors out quickly.
	rdb := redis.NewClient(&redis.Options{
		Addr:        "127.0.0.1:1",
		DialTimeout: 200 * time.Millisecond,
	})
	t.Cleanup(func() { _ = rdb.Close() })
	l := NewRedisScanRateLimiter(rdb, 10, 5)

	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) })
	handler := l.Limit(next)

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, requestAs(1))
	if rec.Code != http.StatusOK {
		t.Fatalf("expected fail-open (200) when redis is unreachable, got %d", rec.Code)
	}
}
