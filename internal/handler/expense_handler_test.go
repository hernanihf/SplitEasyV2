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

type fakeExpenseService struct {
	called bool

	updateErr        error
	deleteErr        error
	getErr           error
	updateCalledWith uint
	deleteCalledWith uint
	getCalledWith    uint
}

func (f *fakeExpenseService) AddExpense(_ context.Context, groupID, paidByID uint, _, category string, amount int64, _ service.SplitMethod, _ []service.SplitInput, _ []service.ItemInput) (*domain.Expense, error) {
	f.called = true
	return &domain.Expense{GroupID: groupID, PaidByID: paidByID, Category: category, Amount: amount}, nil
}

func (f *fakeExpenseService) UpdateExpense(_ context.Context, expenseID, _, paidByID uint, _, category string, amount int64, _ service.SplitMethod, _ []service.SplitInput, _ []service.ItemInput) (*domain.Expense, error) {
	f.updateCalledWith = expenseID
	if f.updateErr != nil {
		return nil, f.updateErr
	}
	return &domain.Expense{ID: expenseID, PaidByID: paidByID, Category: category, Amount: amount}, nil
}

func (f *fakeExpenseService) DeleteExpense(_ context.Context, expenseID, _ uint) error {
	f.deleteCalledWith = expenseID
	return f.deleteErr
}

func (f *fakeExpenseService) GetExpense(_ context.Context, expenseID uint) (*domain.Expense, error) {
	f.getCalledWith = expenseID
	if f.getErr != nil {
		return nil, f.getErr
	}
	return &domain.Expense{ID: expenseID}, nil
}

func (f *fakeExpenseService) GetGroupExpenses(_ context.Context, _ uint) ([]domain.Expense, error) {
	return nil, nil
}

func addExpenseRequest(t *testing.T, authUserID uint, body AddExpenseRequest) *httptest.ResponseRecorder {
	t.Helper()

	payload, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/expenses", bytes.NewReader(payload))
	req = req.WithContext(context.WithValue(req.Context(), middleware.UserIDKey, float64(authUserID)))

	rec := httptest.NewRecorder()
	fake := &fakeExpenseService{}
	h := NewExpenseHandler(fake, fakeGroupServiceForBalance{})
	h.AddExpense(rec, req)
	return rec
}

func TestAddExpense_AllowsCallerAsPayer(t *testing.T) {
	rec := addExpenseRequest(t, 1, AddExpenseRequest{
		GroupID: 1, PaidByID: 1, Description: "Dinner", Amount: 1000,
	})
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestAddExpense_AllowsCallerAsSplitParticipant(t *testing.T) {
	rec := addExpenseRequest(t, 2, AddExpenseRequest{
		GroupID: 1, PaidByID: 1, Description: "Dinner", Amount: 1000,
		SplitMethod: "fixed",
		Splits:      []SplitInputRequest{{UserID: 1, Value: 500}, {UserID: 2, Value: 500}},
	})
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestAddExpense_AllowsEmptySplitsAsImplicitEqualAmongAll(t *testing.T) {
	// No explicit splits means "equal among the whole group", which
	// implicitly includes the caller since they're already a group member.
	rec := addExpenseRequest(t, 2, AddExpenseRequest{
		GroupID: 1, PaidByID: 1, Description: "Dinner", Amount: 1000,
	})
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestAddExpense_RejectsBystanderNotInPaymentOrSplits(t *testing.T) {
	// Caller (3) is neither the payer nor a split participant — fabricating
	// an expense entirely between two other people.
	rec := addExpenseRequest(t, 3, AddExpenseRequest{
		GroupID: 1, PaidByID: 1, Description: "Dinner", Amount: 1000,
		SplitMethod: "fixed",
		Splits:      []SplitInputRequest{{UserID: 2, Value: 1000}},
	})
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d: %s", rec.Code, rec.Body.String())
	}
}

func withURLParam(req *http.Request, key, value string) *http.Request {
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add(key, value)
	return req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
}

func updateExpenseRequest(t *testing.T, fake *fakeExpenseService, expenseID string, authUserID uint, body UpdateExpenseRequest) *httptest.ResponseRecorder {
	t.Helper()

	payload, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}

	req := httptest.NewRequest(http.MethodPut, "/api/v1/expenses/"+expenseID, bytes.NewReader(payload))
	req = req.WithContext(context.WithValue(req.Context(), middleware.UserIDKey, float64(authUserID)))
	req = withURLParam(req, "id", expenseID)

	rec := httptest.NewRecorder()
	h := NewExpenseHandler(fake, fakeGroupServiceForBalance{})
	h.UpdateExpense(rec, req)
	return rec
}

func TestUpdateExpense_Success(t *testing.T) {
	fake := &fakeExpenseService{}
	rec := updateExpenseRequest(t, fake, "5", 1, UpdateExpenseRequest{
		PaidByID: 1, Description: "Dinner (edited)", Amount: 2000,
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if fake.updateCalledWith != 5 {
		t.Fatalf("expected the service to be called with expense id 5, got %d", fake.updateCalledWith)
	}
}

func TestUpdateExpense_MapsNotFoundTo404(t *testing.T) {
	fake := &fakeExpenseService{updateErr: service.ErrExpenseNotFound}
	rec := updateExpenseRequest(t, fake, "5", 1, UpdateExpenseRequest{PaidByID: 1, Amount: 2000})
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestUpdateExpense_MapsNotAPartyTo403(t *testing.T) {
	fake := &fakeExpenseService{updateErr: service.ErrNotExpenseParty}
	rec := updateExpenseRequest(t, fake, "5", 1, UpdateExpenseRequest{PaidByID: 1, Amount: 2000})
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d: %s", rec.Code, rec.Body.String())
	}
}

func deleteExpenseRequest(t *testing.T, fake *fakeExpenseService, expenseID string, authUserID uint) *httptest.ResponseRecorder {
	t.Helper()

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/expenses/"+expenseID, nil)
	req = req.WithContext(context.WithValue(req.Context(), middleware.UserIDKey, float64(authUserID)))
	req = withURLParam(req, "id", expenseID)

	rec := httptest.NewRecorder()
	h := NewExpenseHandler(fake, fakeGroupServiceForBalance{})
	h.DeleteExpense(rec, req)
	return rec
}

func TestDeleteExpense_Success(t *testing.T) {
	fake := &fakeExpenseService{}
	rec := deleteExpenseRequest(t, fake, "7", 1)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d: %s", rec.Code, rec.Body.String())
	}
	if fake.deleteCalledWith != 7 {
		t.Fatalf("expected the service to be called with expense id 7, got %d", fake.deleteCalledWith)
	}
}

func TestDeleteExpense_MapsNotFoundTo404(t *testing.T) {
	fake := &fakeExpenseService{deleteErr: service.ErrExpenseNotFound}
	rec := deleteExpenseRequest(t, fake, "7", 1)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestDeleteExpense_MapsNotAPartyTo403(t *testing.T) {
	fake := &fakeExpenseService{deleteErr: service.ErrNotExpenseParty}
	rec := deleteExpenseRequest(t, fake, "7", 1)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d: %s", rec.Code, rec.Body.String())
	}
}

func getExpenseRequest(t *testing.T, fake *fakeExpenseService, expenseID string, authUserID uint) *httptest.ResponseRecorder {
	t.Helper()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/expenses/"+expenseID, nil)
	req = req.WithContext(context.WithValue(req.Context(), middleware.UserIDKey, float64(authUserID)))
	req = withURLParam(req, "id", expenseID)

	rec := httptest.NewRecorder()
	h := NewExpenseHandler(fake, fakeGroupServiceForBalance{})
	h.GetExpense(rec, req)
	return rec
}

func TestGetExpense_Success(t *testing.T) {
	fake := &fakeExpenseService{}
	rec := getExpenseRequest(t, fake, "11", 1)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if fake.getCalledWith != 11 {
		t.Fatalf("expected the service to be called with expense id 11, got %d", fake.getCalledWith)
	}
}

func TestGetExpense_MapsNotFoundTo404(t *testing.T) {
	fake := &fakeExpenseService{getErr: service.ErrExpenseNotFound}
	rec := getExpenseRequest(t, fake, "11", 1)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", rec.Code, rec.Body.String())
	}
}
