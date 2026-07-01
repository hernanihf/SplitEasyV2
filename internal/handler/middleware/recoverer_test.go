package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRecoverer_RecoversPanicAndReturns500(t *testing.T) {
	panicking := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		panic("boom")
	})
	handler := Recoverer(panicking)

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rec.Code)
	}
	if rec.Body.Len() != 0 {
		t.Errorf("expected no response body (nothing to leak), got %q", rec.Body.String())
	}
}

func TestRecoverer_LetsNonPanickingRequestsThrough(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok")) //nolint:errcheck
	})
	handler := Recoverer(next)

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))

	if rec.Code != http.StatusOK || rec.Body.String() != "ok" {
		t.Fatalf("expected a clean 200/ok passthrough, got %d %q", rec.Code, rec.Body.String())
	}
}

func TestRecoverer_DoesNotRecoverAbortHandler(t *testing.T) {
	defer func() {
		if rvr := recover(); rvr != http.ErrAbortHandler {
			t.Fatalf("expected http.ErrAbortHandler to propagate, got %v", rvr)
		}
	}()

	panicking := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		panic(http.ErrAbortHandler)
	})
	handler := Recoverer(panicking)
	handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))
}
