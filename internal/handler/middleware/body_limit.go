package middleware

import "net/http"

// MaxBytes caps the request body size. Without it, any JSON endpoint will
// happily buffer an arbitrarily large body into memory before decoding it —
// a memory-exhaustion DoS that costs an attacker nothing, and needs no
// account on the two public, pre-auth routes (POST /auth/refresh and
// POST /auth/logout).
//
// http.MaxBytesReader only ever tightens whatever limit already wraps
// r.Body — nesting it doesn't raise the limit. So this must not wrap the
// receipt-scan route, which sets its own, much larger limit for image
// uploads; keep that route on a separate group.
func MaxBytes(limit int64) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			r.Body = http.MaxBytesReader(w, r.Body, limit)
			next.ServeHTTP(w, r)
		})
	}
}
