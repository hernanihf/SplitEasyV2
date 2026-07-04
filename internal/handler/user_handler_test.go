package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"spliteasy/internal/domain"
)

type fakeUserService struct {
	user   *domain.User
	getErr error
}

func (f *fakeUserService) GetUser(_ context.Context, _ uint) (*domain.User, error) {
	if f.getErr != nil {
		return nil, f.getErr
	}
	return f.user, nil
}

func TestSetPushPreference_Success(t *testing.T) {
	fakeUsers := &fakeUserService{}
	fakePush := &fakePushService{}
	h := NewUserHandler(fakeUsers, fakePush)

	body, _ := json.Marshal(SetPushPreferenceRequest{
		PushEnabled:         false,
		PushExpensesEnabled: true,
		PushPaymentsEnabled: false,
		PushCommentsEnabled: true,
	})
	req := authedRequest(http.MethodPatch, "/api/v1/users/me/push-preference", body, 5)
	rec := httptest.NewRecorder()
	h.SetPushPreference(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d: %s", rec.Code, rec.Body.String())
	}
	if fakePush.setPreferencesUserID != 5 ||
		fakePush.setPreferencesEnabled != false ||
		fakePush.setPreferencesExpenses != true ||
		fakePush.setPreferencesPayments != false ||
		fakePush.setPreferencesComments != true {
		t.Errorf("expected SetPushPreferences(5, false, true, false, true), got (%d, %v, %v, %v, %v)",
			fakePush.setPreferencesUserID, fakePush.setPreferencesEnabled,
			fakePush.setPreferencesExpenses, fakePush.setPreferencesPayments, fakePush.setPreferencesComments)
	}
}

func TestSetPushPreference_RejectsUnauthenticated(t *testing.T) {
	h := NewUserHandler(&fakeUserService{}, &fakePushService{})

	body, _ := json.Marshal(SetPushPreferenceRequest{PushEnabled: true})
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/users/me/push-preference", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	h.SetPushPreference(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}

func TestSetPushPreference_RejectsInvalidBody(t *testing.T) {
	h := NewUserHandler(&fakeUserService{}, &fakePushService{})

	req := authedRequest(http.MethodPatch, "/api/v1/users/me/push-preference", []byte("not json"), 5)
	rec := httptest.NewRecorder()
	h.SetPushPreference(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}
