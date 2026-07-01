package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

type fakeAuthService struct {
	refreshCalledWith string
	logoutCalledWith  string
	refreshErr        error
	logoutErr         error
}

func (f *fakeAuthService) GetGoogleLoginURL(_ string) string { return "https://accounts.google.com" }

func (f *fakeAuthService) HandleGoogleCallback(_ context.Context, _ string) (string, string, error) {
	return "access", "refresh", nil
}

func (f *fakeAuthService) RefreshToken(_ context.Context, refreshToken string) (string, string, error) {
	f.refreshCalledWith = refreshToken
	if f.refreshErr != nil {
		return "", "", f.refreshErr
	}
	return "new-access", "new-refresh", nil
}

func (f *fakeAuthService) Logout(_ context.Context, refreshToken string) error {
	f.logoutCalledWith = refreshToken
	return f.logoutErr
}

func jsonBody(t *testing.T, v interface{}) *bytes.Buffer {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal request body: %v", err)
	}
	return bytes.NewBuffer(b)
}

func TestRefresh_ReturnsNewTokens(t *testing.T) {
	fake := &fakeAuthService{}
	h := NewAuthHandler(fake)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/refresh", jsonBody(t, RefreshRequest{RefreshToken: "old-token"}))
	rec := httptest.NewRecorder()
	h.Refresh(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp TokenResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.AccessToken != "new-access" || resp.RefreshToken != "new-refresh" {
		t.Fatalf("unexpected token response: %+v", resp)
	}
	if fake.refreshCalledWith != "old-token" {
		t.Fatalf("expected the service to be called with the request's refresh token, got %q", fake.refreshCalledWith)
	}
}

func TestRefresh_RejectsEmptyToken(t *testing.T) {
	h := NewAuthHandler(&fakeAuthService{})

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/refresh", jsonBody(t, RefreshRequest{RefreshToken: ""}))
	rec := httptest.NewRecorder()
	h.Refresh(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for an empty refresh_token, got %d", rec.Code)
	}
}

func TestRefresh_PropagatesInvalidTokenAs401(t *testing.T) {
	fake := &fakeAuthService{refreshErr: errors.New("invalid or expired refresh token")}
	h := NewAuthHandler(fake)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/refresh", jsonBody(t, RefreshRequest{RefreshToken: "bad-token"}))
	rec := httptest.NewRecorder()
	h.Refresh(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for a rejected refresh token, got %d", rec.Code)
	}
}

func TestLogout_RevokesAndReturnsNoContent(t *testing.T) {
	fake := &fakeAuthService{}
	h := NewAuthHandler(fake)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/logout", jsonBody(t, LogoutRequest{RefreshToken: "some-token"}))
	rec := httptest.NewRecorder()
	h.Logout(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d: %s", rec.Code, rec.Body.String())
	}
	if fake.logoutCalledWith != "some-token" {
		t.Fatalf("expected the service to be called with the request's refresh token, got %q", fake.logoutCalledWith)
	}
}

func TestLogout_RejectsEmptyToken(t *testing.T) {
	h := NewAuthHandler(&fakeAuthService{})

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/logout", jsonBody(t, LogoutRequest{RefreshToken: ""}))
	rec := httptest.NewRecorder()
	h.Logout(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for an empty refresh_token, got %d", rec.Code)
	}
}
