package service

import (
	"context"
	"errors"
	"testing"

	"spliteasy/internal/domain"
)

type fakeCommentRepo struct {
	comments []domain.Comment
	created  []*domain.Comment
	deleted  uint
}

func (f *fakeCommentRepo) Create(_ context.Context, comment *domain.Comment) error {
	comment.ID = uint(len(f.created) + 1)
	f.created = append(f.created, comment)
	return nil
}

func (f *fakeCommentRepo) GetByExpenseID(_ context.Context, expenseID uint) ([]domain.Comment, error) {
	var out []domain.Comment
	for _, c := range f.comments {
		if c.ExpenseID != nil && *c.ExpenseID == expenseID {
			out = append(out, c)
		}
	}
	return out, nil
}

func (f *fakeCommentRepo) GetBySettlementID(_ context.Context, settlementID uint) ([]domain.Comment, error) {
	var out []domain.Comment
	for _, c := range f.comments {
		if c.SettlementID != nil && *c.SettlementID == settlementID {
			out = append(out, c)
		}
	}
	return out, nil
}

func (f *fakeCommentRepo) GetByID(_ context.Context, id uint) (*domain.Comment, error) {
	for _, c := range f.comments {
		if c.ID == id {
			return &c, nil
		}
	}
	return nil, errExpected
}

func (f *fakeCommentRepo) Delete(_ context.Context, id uint) error {
	f.deleted = id
	return nil
}

func TestAddExpenseComment_Success(t *testing.T) {
	repo := &fakeCommentRepo{}
	svc := NewCommentService(repo)

	comment, err := svc.AddExpenseComment(context.Background(), 5, 1, "  looks right to me  ")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if comment.ExpenseID == nil || *comment.ExpenseID != 5 {
		t.Errorf("expected ExpenseID 5, got %+v", comment.ExpenseID)
	}
	if comment.UserID != 1 {
		t.Errorf("expected UserID 1, got %d", comment.UserID)
	}
	if comment.Body != "looks right to me" {
		t.Errorf("expected trimmed body, got %q", comment.Body)
	}
	if len(repo.created) != 1 {
		t.Fatalf("expected comment to be persisted, got %d", len(repo.created))
	}
}

func TestAddExpenseComment_RejectsEmptyBody(t *testing.T) {
	repo := &fakeCommentRepo{}
	svc := NewCommentService(repo)

	_, err := svc.AddExpenseComment(context.Background(), 5, 1, "   ")
	if !errors.Is(err, ErrEmptyComment) {
		t.Errorf("expected ErrEmptyComment, got %v", err)
	}
	if len(repo.created) != 0 {
		t.Errorf("expected no comment to be persisted, got %d", len(repo.created))
	}
}

func TestAddSettlementComment_Success(t *testing.T) {
	repo := &fakeCommentRepo{}
	svc := NewCommentService(repo)

	comment, err := svc.AddSettlementComment(context.Background(), 7, 2, "thanks!")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if comment.SettlementID == nil || *comment.SettlementID != 7 {
		t.Errorf("expected SettlementID 7, got %+v", comment.SettlementID)
	}
	if comment.ExpenseID != nil {
		t.Errorf("expected ExpenseID to stay nil, got %+v", comment.ExpenseID)
	}
}

func TestAddSettlementComment_RejectsEmptyBody(t *testing.T) {
	repo := &fakeCommentRepo{}
	svc := NewCommentService(repo)

	if _, err := svc.AddSettlementComment(context.Background(), 7, 2, ""); !errors.Is(err, ErrEmptyComment) {
		t.Errorf("expected ErrEmptyComment, got %v", err)
	}
}

func TestListExpenseComments_ReturnsOnlyThatExpensesComments(t *testing.T) {
	expenseID, otherID := uint(5), uint(6)
	repo := &fakeCommentRepo{comments: []domain.Comment{
		{ID: 1, ExpenseID: &expenseID, Body: "a"},
		{ID: 2, ExpenseID: &otherID, Body: "b"},
	}}
	svc := NewCommentService(repo)

	comments, err := svc.ListExpenseComments(context.Background(), expenseID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(comments) != 1 || comments[0].ID != 1 {
		t.Errorf("expected only comment 1, got %+v", comments)
	}
}

func TestListSettlementComments_ReturnsOnlyThatSettlementsComments(t *testing.T) {
	settlementID, otherID := uint(7), uint(8)
	repo := &fakeCommentRepo{comments: []domain.Comment{
		{ID: 1, SettlementID: &settlementID, Body: "a"},
		{ID: 2, SettlementID: &otherID, Body: "b"},
	}}
	svc := NewCommentService(repo)

	comments, err := svc.ListSettlementComments(context.Background(), settlementID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(comments) != 1 || comments[0].ID != 1 {
		t.Errorf("expected only comment 1, got %+v", comments)
	}
}

func TestDeleteComment_AllowsAuthor(t *testing.T) {
	repo := &fakeCommentRepo{comments: []domain.Comment{{ID: 3, UserID: 1, Body: "x"}}}
	svc := NewCommentService(repo)

	if err := svc.DeleteComment(context.Background(), 3, 1); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if repo.deleted != 3 {
		t.Errorf("expected Delete to be called with id 3, got %d", repo.deleted)
	}
}

func TestDeleteComment_RejectsNonAuthor(t *testing.T) {
	repo := &fakeCommentRepo{comments: []domain.Comment{{ID: 3, UserID: 1, Body: "x"}}}
	svc := NewCommentService(repo)

	err := svc.DeleteComment(context.Background(), 3, 2)
	if !errors.Is(err, ErrNotCommentAuthor) {
		t.Errorf("expected ErrNotCommentAuthor, got %v", err)
	}
	if repo.deleted != 0 {
		t.Error("expected Delete not to be called")
	}
}

func TestDeleteComment_RejectsUnknownComment(t *testing.T) {
	repo := &fakeCommentRepo{}
	svc := NewCommentService(repo)

	err := svc.DeleteComment(context.Background(), 999, 1)
	if !errors.Is(err, ErrCommentNotFound) {
		t.Errorf("expected ErrCommentNotFound, got %v", err)
	}
}
