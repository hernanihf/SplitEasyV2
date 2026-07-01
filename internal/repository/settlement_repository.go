package repository

import (
	"context"

	"spliteasy/internal/domain"

	"gorm.io/gorm"
)

type SettlementRepository interface {
	Create(ctx context.Context, settlement *domain.Settlement) error
	GetByID(ctx context.Context, id uint) (*domain.Settlement, error)
	GetByGroupID(ctx context.Context, groupID uint) ([]domain.Settlement, error)
	// Delete soft-deletes the settlement (sets deleted_at); it's excluded
	// from every normal query afterward but the row itself is kept.
	Delete(ctx context.Context, id uint) error
}

type settlementRepository struct {
	db *gorm.DB
}

func NewSettlementRepository(db *gorm.DB) SettlementRepository {
	return &settlementRepository{db}
}

func (r *settlementRepository) Create(ctx context.Context, settlement *domain.Settlement) error {
	return r.db.WithContext(ctx).Create(settlement).Error
}

func (r *settlementRepository) GetByID(ctx context.Context, id uint) (*domain.Settlement, error) {
	var settlement domain.Settlement
	if err := r.db.WithContext(ctx).First(&settlement, id).Error; err != nil {
		return nil, err
	}
	return &settlement, nil
}

func (r *settlementRepository) GetByGroupID(ctx context.Context, groupID uint) ([]domain.Settlement, error) {
	var settlements []domain.Settlement
	err := r.db.WithContext(ctx).Where("group_id = ?", groupID).Find(&settlements).Error
	if err != nil {
		return nil, err
	}
	return settlements, nil
}

func (r *settlementRepository) Delete(ctx context.Context, id uint) error {
	return r.db.WithContext(ctx).Delete(&domain.Settlement{}, id).Error
}
