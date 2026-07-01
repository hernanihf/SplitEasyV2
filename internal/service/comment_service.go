package service

import (
	"context"
	"errors"
	"strings"

	"spliteasy/internal/domain"
	"spliteasy/internal/repository"
)

// Sentinel errors that handlers map to HTTP status codes via errors.Is.
var (
	ErrCommentNotFound  = errors.New("comment not found")
	ErrNotCommentAuthor = errors.New("you can only delete your own comment")
	ErrEmptyComment     = errors.New("comment cannot be empty")
)

// CommentService manages comments on expenses and settlements. Group
// membership is checked by the handler (which needs the expense's or
// settlement's group id to do it, via the corresponding Get call) before any
// of these are reached — this service trusts the expenseID/settlementID it's
// given exists and the caller is allowed to see it.
type CommentService interface {
	AddExpenseComment(ctx context.Context, expenseID, userID uint, body string) (*domain.Comment, error)
	AddSettlementComment(ctx context.Context, settlementID, userID uint, body string) (*domain.Comment, error)
	ListExpenseComments(ctx context.Context, expenseID uint) ([]domain.Comment, error)
	ListSettlementComments(ctx context.Context, settlementID uint) ([]domain.Comment, error)
	// DeleteComment soft-deletes a comment. callerID must be the comment's author.
	DeleteComment(ctx context.Context, commentID, callerID uint) error
}

type commentService struct {
	commentRepo repository.CommentRepository
}

func NewCommentService(commentRepo repository.CommentRepository) CommentService {
	return &commentService{commentRepo}
}

func cleanCommentBody(body string) (string, error) {
	body = strings.TrimSpace(body)
	if body == "" {
		return "", ErrEmptyComment
	}
	return body, nil
}

func (s *commentService) AddExpenseComment(ctx context.Context, expenseID, userID uint, body string) (*domain.Comment, error) {
	body, err := cleanCommentBody(body)
	if err != nil {
		return nil, err
	}
	comment := &domain.Comment{ExpenseID: &expenseID, UserID: userID, Body: body}
	if err := s.commentRepo.Create(ctx, comment); err != nil {
		return nil, err
	}
	return comment, nil
}

func (s *commentService) AddSettlementComment(ctx context.Context, settlementID, userID uint, body string) (*domain.Comment, error) {
	body, err := cleanCommentBody(body)
	if err != nil {
		return nil, err
	}
	comment := &domain.Comment{SettlementID: &settlementID, UserID: userID, Body: body}
	if err := s.commentRepo.Create(ctx, comment); err != nil {
		return nil, err
	}
	return comment, nil
}

func (s *commentService) ListExpenseComments(ctx context.Context, expenseID uint) ([]domain.Comment, error) {
	comments, err := s.commentRepo.GetByExpenseID(ctx, expenseID)
	if err != nil {
		return nil, err
	}
	if comments == nil {
		comments = []domain.Comment{}
	}
	return comments, nil
}

func (s *commentService) ListSettlementComments(ctx context.Context, settlementID uint) ([]domain.Comment, error) {
	comments, err := s.commentRepo.GetBySettlementID(ctx, settlementID)
	if err != nil {
		return nil, err
	}
	if comments == nil {
		comments = []domain.Comment{}
	}
	return comments, nil
}

func (s *commentService) DeleteComment(ctx context.Context, commentID, callerID uint) error {
	comment, err := s.commentRepo.GetByID(ctx, commentID)
	if err != nil {
		return ErrCommentNotFound
	}
	if comment.UserID != callerID {
		return ErrNotCommentAuthor
	}
	return s.commentRepo.Delete(ctx, commentID)
}
