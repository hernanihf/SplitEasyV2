package middleware

import (
	"bytes"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRequestLogger_LetsRequestsThrough(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTeapot)
		w.Write([]byte("ok")) //nolint:errcheck
	})
	handler := RequestLogger(next)

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/x", nil))

	if rec.Code != http.StatusTeapot || rec.Body.String() != "ok" {
		t.Fatalf("expected a clean passthrough, got %d %q", rec.Code, rec.Body.String())
	}
}

// The whole reason this middleware exists instead of chi's own Logger: a
// query string (e.g. GET /groups/preview?token=...) must never reach the
// access log, since that's a separate pipe from the app's own slog calls and
// bypasses the RedactingHandler safety net entirely.
func TestRequestLogger_OmitsQueryString(t *testing.T) {
	var buf bytes.Buffer
	restore := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&buf, nil)))
	defer slog.SetDefault(restore)

	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	handler := RequestLogger(next)

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/groups/preview?token=super-secret-token", nil))

	logged := buf.String()
	if strings.Contains(logged, "super-secret-token") {
		t.Errorf("expected the token to be absent from the log line, got: %s", logged)
	}
	if !strings.Contains(logged, "/api/v1/groups/preview") {
		t.Errorf("expected the path (without query) to be logged, got: %s", logged)
	}
}
