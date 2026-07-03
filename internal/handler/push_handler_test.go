package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"spliteasy/internal/handler/middleware"
)

// fakePushService is shared by push_handler_test.go and user_handler_test.go.
type fakePushService struct {
	subscribeErr   error
	unsubscribeErr error

	subscribedUserID                 uint
	subscribedEndpoint               string
	subscribedP256dh, subscribedAuth string
	unsubscribedUserID               uint
	unsubscribedEndpoint             string
	setPushEnabledUserID             uint
	setPushEnabledValue              bool
}

func (f *fakePushService) SetPushEnabled(_ context.Context, userID uint, enabled bool) error {
	f.setPushEnabledUserID = userID
	f.setPushEnabledValue = enabled
	return nil
}

func (f *fakePushService) Subscribe(_ context.Context, userID uint, endpoint, p256dh, auth string) error {
	f.subscribedUserID = userID
	f.subscribedEndpoint = endpoint
	f.subscribedP256dh = p256dh
	f.subscribedAuth = auth
	return f.subscribeErr
}

func (f *fakePushService) Unsubscribe(_ context.Context, userID uint, endpoint string) error {
	f.unsubscribedUserID = userID
	f.unsubscribedEndpoint = endpoint
	return f.unsubscribeErr
}

func (f *fakePushService) NotifyGroupMembers(_ context.Context, _, _ uint, _ func(string) string, _ map[string]string) error {
	return nil
}

func authedRequest(method, path string, body []byte, userID uint) *http.Request {
	req := httptest.NewRequest(method, path, bytes.NewReader(body))
	return req.WithContext(context.WithValue(req.Context(), middleware.UserIDKey, float64(userID)))
}

func TestSubscribe_Success(t *testing.T) {
	fake := &fakePushService{}
	h := NewPushHandler(fake)

	body, _ := json.Marshal(SubscribeRequest{Endpoint: "https://push.example/x", P256dh: "p", Auth: "a"})
	req := authedRequest(http.MethodPost, "/api/v1/push/subscribe", body, 5)
	rec := httptest.NewRecorder()
	h.Subscribe(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d: %s", rec.Code, rec.Body.String())
	}
	if fake.subscribedUserID != 5 || fake.subscribedEndpoint != "https://push.example/x" {
		t.Errorf("unexpected subscribe call: user=%d endpoint=%q", fake.subscribedUserID, fake.subscribedEndpoint)
	}
}

func TestSubscribe_RejectsMissingFields(t *testing.T) {
	fake := &fakePushService{}
	h := NewPushHandler(fake)

	body, _ := json.Marshal(SubscribeRequest{Endpoint: "https://push.example/x"})
	req := authedRequest(http.MethodPost, "/api/v1/push/subscribe", body, 5)
	rec := httptest.NewRecorder()
	h.Subscribe(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestSubscribe_PropagatesServiceError(t *testing.T) {
	fake := &fakePushService{subscribeErr: errors.New("cap reached")}
	h := NewPushHandler(fake)

	body, _ := json.Marshal(SubscribeRequest{Endpoint: "https://push.example/x", P256dh: "p", Auth: "a"})
	req := authedRequest(http.MethodPost, "/api/v1/push/subscribe", body, 5)
	rec := httptest.NewRecorder()
	h.Subscribe(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestUnsubscribe_Success(t *testing.T) {
	fake := &fakePushService{}
	h := NewPushHandler(fake)

	body, _ := json.Marshal(UnsubscribeRequest{Endpoint: "https://push.example/x"})
	req := authedRequest(http.MethodDelete, "/api/v1/push/subscribe", body, 5)
	rec := httptest.NewRecorder()
	h.Unsubscribe(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d: %s", rec.Code, rec.Body.String())
	}
	if fake.unsubscribedUserID != 5 || fake.unsubscribedEndpoint != "https://push.example/x" {
		t.Errorf("unexpected unsubscribe call: user=%d endpoint=%q", fake.unsubscribedUserID, fake.unsubscribedEndpoint)
	}
}

func TestUnsubscribe_RejectsMissingEndpoint(t *testing.T) {
	fake := &fakePushService{}
	h := NewPushHandler(fake)

	body, _ := json.Marshal(UnsubscribeRequest{})
	req := authedRequest(http.MethodDelete, "/api/v1/push/subscribe", body, 5)
	rec := httptest.NewRecorder()
	h.Unsubscribe(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}
