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
	"spliteasy/internal/service"

	"github.com/go-chi/chi/v5"
)

type fakeBalanceService struct {
	deleteSettlementErr        error
	deleteSettlementCalledWith uint
	getSettlementErr           error
	getSettlementCalledWith    uint
}

func (*fakeBalanceService) CalculateGroupDebts(_ context.Context, _ uint) ([]domain.Debt, error) {
	return nil, nil
}

func (*fakeBalanceService) SettleDebt(_ context.Context, _, from, to uint, amount int64) (*domain.Settlement, error) {
	return &domain.Settlement{FromUserID: from, ToUserID: to, Amount: amount}, nil
}

func (*fakeBalanceService) ListSettlements(_ context.Context, _ uint) ([]domain.Settlement, error) {
	return nil, nil
}

func (f *fakeBalanceService) GetSettlement(_ context.Context, settlementID uint) (*domain.Settlement, error) {
	f.getSettlementCalledWith = settlementID
	if f.getSettlementErr != nil {
		return nil, f.getSettlementErr
	}
	return &domain.Settlement{ID: settlementID}, nil
}

func (f *fakeBalanceService) DeleteSettlement(_ context.Context, settlementID, _ uint) error {
	f.deleteSettlementCalledWith = settlementID
	return f.deleteSettlementErr
}

type fakeGroupServiceForBalance struct{}

func (fakeGroupServiceForBalance) CreateGroup(_ context.Context, _, _, _ string, _ uint) (*domain.Group, error) {
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
	h := NewBalanceHandler(&fakeBalanceService{}, fakeGroupServiceForBalance{})
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

func deleteSettlementRequest(t *testing.T, fake *fakeBalanceService, settlementID string, authUserID uint) *httptest.ResponseRecorder {
	t.Helper()

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/settlements/"+settlementID, nil)
	req = req.WithContext(context.WithValue(req.Context(), middleware.UserIDKey, float64(authUserID)))

	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", settlementID)
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	rec := httptest.NewRecorder()
	h := NewBalanceHandler(fake, fakeGroupServiceForBalance{})
	h.DeleteSettlement(rec, req)
	return rec
}

func TestDeleteSettlement_Success(t *testing.T) {
	fake := &fakeBalanceService{}
	rec := deleteSettlementRequest(t, fake, "9", 1)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d: %s", rec.Code, rec.Body.String())
	}
	if fake.deleteSettlementCalledWith != 9 {
		t.Fatalf("expected the service to be called with settlement id 9, got %d", fake.deleteSettlementCalledWith)
	}
}

func TestDeleteSettlement_MapsNotFoundTo404(t *testing.T) {
	fake := &fakeBalanceService{deleteSettlementErr: service.ErrSettlementNotFound}
	rec := deleteSettlementRequest(t, fake, "9", 1)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestDeleteSettlement_MapsNotAPartyTo403(t *testing.T) {
	fake := &fakeBalanceService{deleteSettlementErr: service.ErrNotSettlementParty}
	rec := deleteSettlementRequest(t, fake, "9", 1)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d: %s", rec.Code, rec.Body.String())
	}
}

func getSettlementRequest(t *testing.T, fake *fakeBalanceService, settlementID string, authUserID uint) *httptest.ResponseRecorder {
	t.Helper()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/settlements/"+settlementID, nil)
	req = req.WithContext(context.WithValue(req.Context(), middleware.UserIDKey, float64(authUserID)))

	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", settlementID)
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	rec := httptest.NewRecorder()
	h := NewBalanceHandler(fake, fakeGroupServiceForBalance{})
	h.GetSettlement(rec, req)
	return rec
}

func TestGetSettlement_Success(t *testing.T) {
	fake := &fakeBalanceService{}
	rec := getSettlementRequest(t, fake, "12", 1)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if fake.getSettlementCalledWith != 12 {
		t.Fatalf("expected the service to be called with settlement id 12, got %d", fake.getSettlementCalledWith)
	}
}

func TestGetSettlement_MapsNotFoundTo404(t *testing.T) {
	fake := &fakeBalanceService{getSettlementErr: service.ErrSettlementNotFound}
	rec := getSettlementRequest(t, fake, "12", 1)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", rec.Code, rec.Body.String())
	}
}
