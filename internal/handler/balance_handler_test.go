package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"spliteasy/internal/domain"
	"spliteasy/internal/handler/middleware"

	"github.com/go-chi/chi/v5"
)

type fakeBalanceService struct{}

func (fakeBalanceService) CalculateGroupDebts(_ context.Context, _ uint) ([]domain.Debt, error) {
	return nil, nil
}

func (fakeBalanceService) SettleDebt(_ context.Context, _, from, to uint, amount int64) (*domain.Settlement, error) {
	return &domain.Settlement{FromUserID: from, ToUserID: to, Amount: amount}, nil
}

func (fakeBalanceService) ListSettlements(_ context.Context, _ uint) ([]domain.Settlement, error) {
	return nil, nil
}

type fakeGroupServiceForBalance struct{}

func (fakeGroupServiceForBalance) CreateGroup(_ context.Context, _, _ string, _ uint) (*domain.Group, error) {
	return nil, nil
}

func (fakeGroupServiceForBalance) GetGroup(_ context.Context, _ uint) (*domain.Group, error) {
	return nil, nil
}

func (fakeGroupServiceForBalance) ListGroupsForUser(_ context.Context, _ uint) ([]domain.Group, error) {
	return nil, nil
}

func (fakeGroupServiceForBalance) GetInviteToken(_ context.Context, _, _ uint) (string, error) {
	return "", nil
}

func (fakeGroupServiceForBalance) JoinGroup(_ context.Context, _ string, _ uint) (*domain.Group, error) {
	return nil, nil
}

func (fakeGroupServiceForBalance) VerifyMembership(_ context.Context, _, _ uint) error {
	return nil
}

// settleDebtRequest builds and executes a SettleDebt call as if it went
// through JWTAuth (which stores the user id as a float64, matching how JWT
// numeric claims decode) and chi's URL param routing for "id".
func settleDebtRequest(t *testing.T, authUserID uint, body SettleDebtRequest) *httptest.ResponseRecorder {
	t.Helper()

	payload, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/groups/1/settlements", bytes.NewReader(payload))
	req = req.WithContext(context.WithValue(req.Context(), middleware.UserIDKey, float64(authUserID)))

	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", "1")
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	rec := httptest.NewRecorder()
	h := NewBalanceHandler(fakeBalanceService{}, fakeGroupServiceForBalance{})
	h.SettleDebt(rec, req)
	return rec
}

func TestSettleDebt_AllowsPayerToRecordIt(t *testing.T) {
	rec := settleDebtRequest(t, 2, SettleDebtRequest{FromUserID: 2, ToUserID: 1, Amount: 500})
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestSettleDebt_AllowsPayeeToRecordIt(t *testing.T) {
	rec := settleDebtRequest(t, 1, SettleDebtRequest{FromUserID: 2, ToUserID: 1, Amount: 500})
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestSettleDebt_RejectsBystander(t *testing.T) {
	rec := settleDebtRequest(t, 3, SettleDebtRequest{FromUserID: 2, ToUserID: 1, Amount: 500})
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d: %s", rec.Code, rec.Body.String())
	}
}
