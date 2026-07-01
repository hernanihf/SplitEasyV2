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
)

type fakeExpenseService struct {
	called bool
}

func (f *fakeExpenseService) AddExpense(_ context.Context, groupID, paidByID uint, _ string, amount int64, _ service.SplitMethod, _ []service.SplitInput, _ []service.ItemInput) (*domain.Expense, error) {
	f.called = true
	return &domain.Expense{GroupID: groupID, PaidByID: paidByID, Amount: amount}, nil
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
