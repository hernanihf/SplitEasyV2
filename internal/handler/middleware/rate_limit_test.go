package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"runtime"
	"testing"
	"time"
)

func TestScanRateLimiter_BurstThenDeny(t *testing.T) {
	l := NewScanRateLimiter(10, 2) // burst of 2
	defer l.Close()

	if ok, _ := l.allow(1); !ok {
		t.Fatal("first scan should be allowed")
	}
	if ok, _ := l.allow(1); !ok {
		t.Fatal("second scan should be allowed (within burst)")
	}
	if ok, _ := l.allow(1); ok {
		t.Fatal("third scan should be denied")
	}
	// A different user has their own bucket.
	if ok, _ := l.allow(2); !ok {
		t.Fatal("a different user should not be affected")
	}
}

func TestScanRateLimiter_Refills(t *testing.T) {
	l := NewScanRateLimiter(3600, 1) // 1 token/sec, burst 1
	defer l.Close()

	if ok, _ := l.allow(1); !ok {
		t.Fatal("first scan should be allowed")
	}
	if ok, _ := l.allow(1); ok {
		t.Fatal("immediate second scan should be denied")
	}

	// Simulate time passing by rewinding the bucket's last-refill timestamp.
	l.mu.Lock()
	l.buckets[1].last = l.buckets[1].last.Add(-2 * time.Second)
	l.mu.Unlock()

	if ok, _ := l.allow(1); !ok {
		t.Fatal("scan should be allowed again after refill")
	}
}

func TestScanRateLimiter_MiddlewareReturns429(t *testing.T) {
	l := NewScanRateLimiter(10, 1) // burst 1
	defer l.Close()
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) })
	handler := l.Limit(next)

	newReq := func() *http.Request {
		r := httptest.NewRequest(http.MethodPost, "/receipts/scan", nil)
		return r.WithContext(context.WithValue(r.Context(), UserIDKey, float64(7)))
	}

	first := httptest.NewRecorder()
	handler.ServeHTTP(first, newReq())
	if first.Code != http.StatusOK {
		t.Fatalf("expected 200 on first request, got %d", first.Code)
	}

	second := httptest.NewRecorder()
	handler.ServeHTTP(second, newReq())
	if second.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429 on second request, got %d", second.Code)
	}
	if second.Header().Get("Retry-After") == "" {
		t.Error("expected a Retry-After header on the 429 response")
	}
}

func TestScanRateLimiter_CloseStopsCleanupGoroutine(t *testing.T) {
	before := runtime.NumGoroutine()

	l := NewScanRateLimiter(10, 5)
	if ok, _ := l.allow(1); !ok {
		t.Fatal("expected the limiter to work before Close")
	}
	l.Close()

	// cleanupLoop should exit promptly once stop is closed; poll briefly
	// instead of asserting on a single runtime.NumGoroutine() snapshot,
	// which is inherently racy against the scheduler.
	deadline := time.Now().Add(1 * time.Second)
	for runtime.NumGoroutine() > before {
		if time.Now().After(deadline) {
			t.Fatalf("cleanup goroutine did not exit after Close (before=%d, now=%d)", before, runtime.NumGoroutine())
		}
		time.Sleep(10 * time.Millisecond)
	}
}
