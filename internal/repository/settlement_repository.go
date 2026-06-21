package repository

import (
	"spliteasy/internal/domain"

	"gorm.io/gorm"
)

type SettlementRepository interface {
	Create(settlement *domain.Settlement) error
	GetByGroupID(groupID uint) ([]domain.Settlement, error)
}

type settlementRepository struct {
	db *gorm.DB
}

func NewSettlementRepository(db *gorm.DB) SettlementRepository {
	return &settlementRepository{db}
}

func (r *settlementRepository) Create(settlement *domain.Settlement) error {
	return r.db.Create(settlement).Error
}

func (r *settlementRepository) GetByGroupID(groupID uint) ([]domain.Settlement, error) {
	var settlements []domain.Settlement
	err := r.db.Where("group_id = ?", groupID).Find(&settlements).Error
	if err != nil {
		return nil, err
	}
	return settlements, nil
}
