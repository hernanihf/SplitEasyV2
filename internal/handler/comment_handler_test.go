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

type fakeCommentService struct {
	addExpenseErr     error
	addSettlementErr  error
	listExpenseErr    error
	listSettlementErr error
	deleteErr         error

	addExpenseCalledWith     uint
	addSettlementCalledWith  uint
	listExpenseCalledWith    uint
	listSettlementCalledWith uint
	deleteCalledWith         uint
}

func (f *fakeCommentService) AddExpenseComment(_ context.Context, expenseID, userID uint, body string) (*domain.Comment, error) {
	f.addExpenseCalledWith = expenseID
	if f.addExpenseErr != nil {
		return nil, f.addExpenseErr
	}
	return &domain.Comment{ID: 1, ExpenseID: &expenseID, UserID: userID, Body: body}, nil
}

func (f *fakeCommentService) AddSettlementComment(_ context.Context, settlementID, userID uint, body string) (*domain.Comment, error) {
	f.addSettlementCalledWith = settlementID
	if f.addSettlementErr != nil {
		return nil, f.addSettlementErr
	}
	return &domain.Comment{ID: 1, SettlementID: &settlementID, UserID: userID, Body: body}, nil
}

func (f *fakeCommentService) ListExpenseComments(_ context.Context, expenseID uint) ([]domain.Comment, error) {
	f.listExpenseCalledWith = expenseID
	if f.listExpenseErr != nil {
		return nil, f.listExpenseErr
	}
	return []domain.Comment{}, nil
}

func (f *fakeCommentService) ListSettlementComments(_ context.Context, settlementID uint) ([]domain.Comment, error) {
	f.listSettlementCalledWith = settlementID
	if f.listSettlementErr != nil {
		return nil, f.listSettlementErr
	}
	return []domain.Comment{}, nil
}

func (f *fakeCommentService) DeleteComment(_ context.Context, commentID, _ uint) error {
	f.deleteCalledWith = commentID
	return f.deleteErr
}

// newCommentHandler wires a CommentHandler with sane fake defaults, letting
// each test only supply the fakes it actually cares about.
func newCommentHandler(commentSvc *fakeCommentService, expenseSvc *fakeExpenseService, balanceSvc *fakeBalanceService) *CommentHandler {
	if expenseSvc == nil {
		expenseSvc = &fakeExpenseService{}
	}
	if balanceSvc == nil {
		balanceSvc = &fakeBalanceService{}
	}
	return NewCommentHandler(commentSvc, expenseSvc, balanceSvc, fakeGroupServiceForBalance{})
}

func addExpenseCommentRequest(t *testing.T, h *CommentHandler, expenseID string, authUserID uint, body AddCommentRequest) *httptest.ResponseRecorder {
	t.Helper()

	payload, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/expenses/"+expenseID+"/comments", bytes.NewReader(payload))
	req = req.WithContext(context.WithValue(req.Context(), middleware.UserIDKey, float64(authUserID)))
	req = withURLParam(req, "id", expenseID)

	rec := httptest.NewRecorder()
	h.AddExpenseComment(rec, req)
	return rec
}

func TestAddExpenseComment_Success(t *testing.T) {
	commentSvc := &fakeCommentService{}
	h := newCommentHandler(commentSvc, nil, nil)
	rec := addExpenseCommentRequest(t, h, "5", 1, AddCommentRequest{Body: "hi"})
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}
	if commentSvc.addExpenseCalledWith != 5 {
		t.Fatalf("expected the service to be called with expense id 5, got %d", commentSvc.addExpenseCalledWith)
	}
}

func TestAddExpenseComment_MapsExpenseNotFoundTo404(t *testing.T) {
	expenseSvc := &fakeExpenseService{getErr: service.ErrExpenseNotFound}
	h := newCommentHandler(&fakeCommentService{}, expenseSvc, nil)
	rec := addExpenseCommentRequest(t, h, "5", 1, AddCommentRequest{Body: "hi"})
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestAddExpenseComment_RejectsEmptyBody(t *testing.T) {
	commentSvc := &fakeCommentService{addExpenseErr: service.ErrEmptyComment}
	h := newCommentHandler(commentSvc, nil, nil)
	rec := addExpenseCommentRequest(t, h, "5", 1, AddCommentRequest{Body: "   "})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
}

func listExpenseCommentsRequest(t *testing.T, h *CommentHandler, expenseID string, authUserID uint) *httptest.ResponseRecorder {
	t.Helper()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/expenses/"+expenseID+"/comments", nil)
	req = req.WithContext(context.WithValue(req.Context(), middleware.UserIDKey, float64(authUserID)))
	req = withURLParam(req, "id", expenseID)

	rec := httptest.NewRecorder()
	h.ListExpenseComments(rec, req)
	return rec
}

func TestListExpenseComments_Success(t *testing.T) {
	commentSvc := &fakeCommentService{}
	h := newCommentHandler(commentSvc, nil, nil)
	rec := listExpenseCommentsRequest(t, h, "5", 1)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if commentSvc.listExpenseCalledWith != 5 {
		t.Fatalf("expected the service to be called with expense id 5, got %d", commentSvc.listExpenseCalledWith)
	}
}

func addSettlementCommentRequest(t *testing.T, h *CommentHandler, settlementID string, authUserID uint, body AddCommentRequest) *httptest.ResponseRecorder {
	t.Helper()

	payload, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/settlements/"+settlementID+"/comments", bytes.NewReader(payload))
	req = req.WithContext(context.WithValue(req.Context(), middleware.UserIDKey, float64(authUserID)))
	req = withURLParam(req, "id", settlementID)

	rec := httptest.NewRecorder()
	h.AddSettlementComment(rec, req)
	return rec
}

func TestAddSettlementComment_Success(t *testing.T) {
	commentSvc := &fakeCommentService{}
	h := newCommentHandler(commentSvc, nil, nil)
	rec := addSettlementCommentRequest(t, h, "9", 1, AddCommentRequest{Body: "hi"})
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}
	if commentSvc.addSettlementCalledWith != 9 {
		t.Fatalf("expected the service to be called with settlement id 9, got %d", commentSvc.addSettlementCalledWith)
	}
}

func TestAddSettlementComment_MapsSettlementNotFoundTo404(t *testing.T) {
	balanceSvc := &fakeBalanceService{getSettlementErr: service.ErrSettlementNotFound}
	h := newCommentHandler(&fakeCommentService{}, nil, balanceSvc)
	rec := addSettlementCommentRequest(t, h, "9", 1, AddCommentRequest{Body: "hi"})
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", rec.Code, rec.Body.String())
	}
}

func listSettlementCommentsRequest(t *testing.T, h *CommentHandler, settlementID string, authUserID uint) *httptest.ResponseRecorder {
	t.Helper()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/settlements/"+settlementID+"/comments", nil)
	req = req.WithContext(context.WithValue(req.Context(), middleware.UserIDKey, float64(authUserID)))
	req = withURLParam(req, "id", settlementID)

	rec := httptest.NewRecorder()
	h.ListSettlementComments(rec, req)
	return rec
}

func TestListSettlementComments_Success(t *testing.T) {
	commentSvc := &fakeCommentService{}
	h := newCommentHandler(commentSvc, nil, nil)
	rec := listSettlementCommentsRequest(t, h, "9", 1)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if commentSvc.listSettlementCalledWith != 9 {
		t.Fatalf("expected the service to be called with settlement id 9, got %d", commentSvc.listSettlementCalledWith)
	}
}

func deleteCommentRequest(t *testing.T, h *CommentHandler, commentID string, authUserID uint) *httptest.ResponseRecorder {
	t.Helper()

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/comments/"+commentID, nil)
	req = req.WithContext(context.WithValue(req.Context(), middleware.UserIDKey, float64(authUserID)))
	req = withURLParam(req, "id", commentID)

	rec := httptest.NewRecorder()
	h.DeleteComment(rec, req)
	return rec
}

func TestDeleteComment_Success(t *testing.T) {
	commentSvc := &fakeCommentService{}
	h := newCommentHandler(commentSvc, nil, nil)
	rec := deleteCommentRequest(t, h, "3", 1)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d: %s", rec.Code, rec.Body.String())
	}
	if commentSvc.deleteCalledWith != 3 {
		t.Fatalf("expected the service to be called with comment id 3, got %d", commentSvc.deleteCalledWith)
	}
}

func TestDeleteComment_MapsNotFoundTo404(t *testing.T) {
	commentSvc := &fakeCommentService{deleteErr: service.ErrCommentNotFound}
	h := newCommentHandler(commentSvc, nil, nil)
	rec := deleteCommentRequest(t, h, "3", 1)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestDeleteComment_MapsNotAuthorTo403(t *testing.T) {
	commentSvc := &fakeCommentService{deleteErr: service.ErrNotCommentAuthor}
	h := newCommentHandler(commentSvc, nil, nil)
	rec := deleteCommentRequest(t, h, "3", 1)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d: %s", rec.Code, rec.Body.String())
	}
}
