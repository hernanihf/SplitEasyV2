package middleware

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestMaxBytes_AllowsBodyUnderLimit(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("unexpected read error: %v", err)
		}
		w.Write(body) //nolint:errcheck
	})
	handler := MaxBytes(10)(next)

	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader([]byte("12345")))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Body.String() != "12345" {
		t.Errorf("expected body to pass through unchanged, got %q", rec.Body.String())
	}
}

func TestMaxBytes_RejectsBodyOverLimit(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, err := io.ReadAll(r.Body)
		if err == nil {
			t.Error("expected a read error once the body exceeds the limit")
		}
		w.WriteHeader(http.StatusBadRequest)
	})
	handler := MaxBytes(10)(next)

	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader([]byte("this is way more than ten bytes")))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected the handler to observe the read error, got status %d", rec.Code)
	}
}
