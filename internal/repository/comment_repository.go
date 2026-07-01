package repository

import (
	"context"

	"spliteasy/internal/domain"

	"gorm.io/gorm"
)

type CommentRepository interface {
	Create(ctx context.Context, comment *domain.Comment) error
	GetByExpenseID(ctx context.Context, expenseID uint) ([]domain.Comment, error)
	GetBySettlementID(ctx context.Context, settlementID uint) ([]domain.Comment, error)
	GetByID(ctx context.Context, id uint) (*domain.Comment, error)
	// Delete soft-deletes the comment (sets deleted_at); it's excluded from
	// every normal query afterward but the row itself is kept.
	Delete(ctx context.Context, id uint) error
}

type commentRepository struct {
	db *gorm.DB
}

func NewCommentRepository(db *gorm.DB) CommentRepository {
	return &commentRepository{db}
}

// Create inserts the comment, then reloads it with User preloaded so the
// caller gets back the commenter's name/avatar without a second round trip.
// Omitting the User association on Create avoids GORM trying to upsert a
// (zero-value) user row from the comment's unset User field.
func (r *commentRepository) Create(ctx context.Context, comment *domain.Comment) error {
	if err := r.db.WithContext(ctx).Omit("User").Create(comment).Error; err != nil {
		return err
	}
	return r.db.WithContext(ctx).Preload("User").First(comment, comment.ID).Error
}

func (r *commentRepository) GetByExpenseID(ctx context.Context, expenseID uint) ([]domain.Comment, error) {
	var comments []domain.Comment
	err := r.db.WithContext(ctx).
		Preload("User").
		Where("expense_id = ?", expenseID).
		Order("created_at asc").
		Find(&comments).Error
	if err != nil {
		return nil, err
	}
	return comments, nil
}

func (r *commentRepository) GetBySettlementID(ctx context.Context, settlementID uint) ([]domain.Comment, error) {
	var comments []domain.Comment
	err := r.db.WithContext(ctx).
		Preload("User").
		Where("settlement_id = ?", settlementID).
		Order("created_at asc").
		Find(&comments).Error
	if err != nil {
		return nil, err
	}
	return comments, nil
}

func (r *commentRepository) GetByID(ctx context.Context, id uint) (*domain.Comment, error) {
	var comment domain.Comment
	if err := r.db.WithContext(ctx).First(&comment, id).Error; err != nil {
		return nil, err
	}
	return &comment, nil
}

func (r *commentRepository) Delete(ctx context.Context, id uint) error {
	return r.db.WithContext(ctx).Delete(&domain.Comment{}, id).Error
}
