package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// StorageService persists files to Supabase Storage. Only the receipt image
// flow uses this today, but it's kept generic (bucket/path are caller-
// supplied) rather than receipt-specific.
type StorageService interface {
	// Upload writes data to bucket/path, upserting if it already exists.
	Upload(ctx context.Context, path string, data []byte, contentType string) error
	// SignedURL returns a short-lived, publicly-fetchable URL for a private
	// object — generated fresh on every call, never cached, since a stored
	// URL would eventually expire while a stored *path* never does.
	SignedURL(ctx context.Context, path string, expiry time.Duration) (string, error)
}

type storageService struct {
	httpClient httpDoer
	baseURL    string // e.g. https://<project-ref>.supabase.co
	serviceKey string
	bucket     string
}

// NewStorageService returns a StorageService, or nil if Supabase Storage
// isn't configured (empty baseURL/serviceKey) — callers must check for nil
// and degrade gracefully (see receipt_service.go), since receipt image
// persistence is additive and shouldn't block the app from starting or the
// AI scan from working if storage isn't set up yet.
func NewStorageService(httpClient httpDoer, baseURL, serviceKey, bucket string) StorageService {
	if baseURL == "" || serviceKey == "" {
		return nil
	}
	return &storageService{httpClient, strings.TrimSuffix(baseURL, "/"), serviceKey, bucket}
}

func (s *storageService) Upload(ctx context.Context, path string, data []byte, contentType string) error {
	url := fmt.Sprintf("%s/storage/v1/object/%s/%s", s.baseURL, s.bucket, path)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+s.serviceKey)
	req.Header.Set("Content-Type", contentType)
	// Upsert so a retry after a network blip overwrites rather than 409s —
	// each object's path is a fresh UUID anyway, so collisions are moot.
	req.Header.Set("x-upsert", "true")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("storage upload request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("storage upload returned status %d", resp.StatusCode)
	}
	return nil
}

func (s *storageService) SignedURL(ctx context.Context, path string, expiry time.Duration) (string, error) {
	url := fmt.Sprintf("%s/storage/v1/object/sign/%s/%s", s.baseURL, s.bucket, path)
	payload, err := json.Marshal(map[string]int{"expiresIn": int(expiry.Seconds())})
	if err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+s.serviceKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("storage sign request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("storage sign returned status %d", resp.StatusCode)
	}

	var signed struct {
		SignedURL string `json:"signedURL"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&signed); err != nil {
		return "", fmt.Errorf("failed to decode sign response: %w", err)
	}
	if signed.SignedURL == "" {
		return "", errors.New("storage sign response had no signedURL")
	}

	// Supabase returns a path like "/object/sign/{bucket}/{path}?token=...",
	// not a full URL — stitch the storage base back on.
	if strings.HasPrefix(signed.SignedURL, "/") {
		return s.baseURL + "/storage/v1" + signed.SignedURL, nil
	}
	return signed.SignedURL, nil
}
