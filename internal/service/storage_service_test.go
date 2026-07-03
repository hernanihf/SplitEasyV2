package service

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestNewStorageService_ReturnsNilWhenUnconfigured(t *testing.T) {
	if svc := NewStorageService(&fakeHTTPDoer{}, "", "key", "bucket"); svc != nil {
		t.Error("expected nil when baseURL is empty")
	}
	if svc := NewStorageService(&fakeHTTPDoer{}, "https://example.supabase.co", "", "bucket"); svc != nil {
		t.Error("expected nil when serviceKey is empty")
	}
}

func TestStorageService_Upload_SendsAuthAndUpsertHeaders(t *testing.T) {
	doer := &fakeHTTPDoer{response: jsonResponse(http.StatusOK, "")}
	svc := NewStorageService(doer, "https://example.supabase.co", "service-key", "receipts")

	err := svc.Upload(context.Background(), "abc123.jpg", []byte("fake-image-bytes"), "image/jpeg")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if doer.lastReq.Header.Get("Authorization") != "Bearer service-key" {
		t.Errorf("expected Authorization header, got %q", doer.lastReq.Header.Get("Authorization"))
	}
	if doer.lastReq.Header.Get("x-upsert") != "true" {
		t.Errorf("expected x-upsert header to be true, got %q", doer.lastReq.Header.Get("x-upsert"))
	}
	if doer.lastReq.Header.Get("Content-Type") != "image/jpeg" {
		t.Errorf("expected Content-Type image/jpeg, got %q", doer.lastReq.Header.Get("Content-Type"))
	}
	wantURL := "https://example.supabase.co/storage/v1/object/receipts/abc123.jpg"
	if doer.lastReq.URL.String() != wantURL {
		t.Errorf("expected URL %q, got %q", wantURL, doer.lastReq.URL.String())
	}
}

func TestStorageService_Upload_ReturnsErrorOnNonOKStatus(t *testing.T) {
	doer := &fakeHTTPDoer{response: jsonResponse(http.StatusForbidden, "")}
	svc := NewStorageService(doer, "https://example.supabase.co", "service-key", "receipts")

	if err := svc.Upload(context.Background(), "abc123.jpg", []byte("data"), "image/jpeg"); err == nil {
		t.Error("expected error on non-2xx status")
	}
}

func TestStorageService_SignedURL_StitchesRelativePathOntoBaseURL(t *testing.T) {
	body := `{"signedURL":"/object/sign/receipts/abc123.jpg?token=xyz"}`
	doer := &fakeHTTPDoer{response: jsonResponse(http.StatusOK, body)}
	svc := NewStorageService(doer, "https://example.supabase.co", "service-key", "receipts")

	url, err := svc.SignedURL(context.Background(), "abc123.jpg", 10*time.Minute)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := "https://example.supabase.co/storage/v1/object/sign/receipts/abc123.jpg?token=xyz"
	if url != want {
		t.Errorf("expected %q, got %q", want, url)
	}

	sentBody, _ := io.ReadAll(doer.lastReq.Body)
	if !strings.Contains(string(sentBody), `"expiresIn":600`) {
		t.Errorf("expected expiresIn 600 seconds in request body, got: %s", sentBody)
	}
}

func TestStorageService_SignedURL_ReturnsAbsoluteURLAsIs(t *testing.T) {
	body := `{"signedURL":"https://cdn.example.com/abc123.jpg?token=xyz"}`
	doer := &fakeHTTPDoer{response: jsonResponse(http.StatusOK, body)}
	svc := NewStorageService(doer, "https://example.supabase.co", "service-key", "receipts")

	url, err := svc.SignedURL(context.Background(), "abc123.jpg", 10*time.Minute)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if url != "https://cdn.example.com/abc123.jpg?token=xyz" {
		t.Errorf("expected absolute URL to pass through unchanged, got %q", url)
	}
}

func TestStorageService_SignedURL_ReturnsErrorOnMissingSignedURL(t *testing.T) {
	doer := &fakeHTTPDoer{response: jsonResponse(http.StatusOK, `{}`)}
	svc := NewStorageService(doer, "https://example.supabase.co", "service-key", "receipts")

	if _, err := svc.SignedURL(context.Background(), "abc123.jpg", 10*time.Minute); err == nil {
		t.Error("expected error when response has no signedURL")
	}
}

func TestStorageService_SignedURL_ReturnsErrorOnNonOKStatus(t *testing.T) {
	doer := &fakeHTTPDoer{response: jsonResponse(http.StatusNotFound, "")}
	svc := NewStorageService(doer, "https://example.supabase.co", "service-key", "receipts")

	if _, err := svc.SignedURL(context.Background(), "abc123.jpg", 10*time.Minute); err == nil {
		t.Error("expected error on non-2xx status")
	}
}
