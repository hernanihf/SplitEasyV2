package repository

import (
	"context"

	"spliteasy/internal/domain"

	"gorm.io/gorm"
)

type SettlementRepository interface {
	Create(ctx context.Context, settlement *domain.Settlement) error
	GetByGroupID(ctx context.Context, groupID uint) ([]domain.Settlement, error)
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

func (r *settlementRepository) GetByGroupID(ctx context.Context, groupID uint) ([]domain.Settlement, error) {
	var settlements []domain.Settlement
	err := r.db.WithContext(ctx).Where("group_id = ?", groupID).Find(&settlements).Error
	if err != nil {
		return nil, err
	}
	return settlements, nil
}
