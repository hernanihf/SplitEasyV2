package repository

import (
	"context"

	"spliteasy/internal/domain"

	"gorm.io/gorm"
)

type ExpenseRepository interface {
	CreateWithSplits(ctx context.Context, expense *domain.Expense, splits []domain.ExpenseSplit) error
	GetByGroupID(ctx context.Context, groupID uint) ([]domain.Expense, error)
}

type expenseRepository struct {
	db *gorm.DB
}

func NewExpenseRepository(db *gorm.DB) ExpenseRepository {
	return &expenseRepository{db}
}

// CreateWithSplits creates an expense and its related splits in a single transaction
func (r *expenseRepository) CreateWithSplits(ctx context.Context, expense *domain.Expense, splits []domain.ExpenseSplit) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Insert expense
		if err := tx.Create(expense).Error; err != nil {
			return err
		}

		// Assign ExpenseID to all splits
		for i := range splits {
			splits[i].ExpenseID = expense.ID
		}

		// Insert all splits
		if err := tx.Create(&splits).Error; err != nil {
			return err
		}

		return nil
	})
}

func (r *expenseRepository) GetByGroupID(ctx context.Context, groupID uint) ([]domain.Expense, error) {
	var expenses []domain.Expense
	err := r.db.WithContext(ctx).Preload("Splits").Preload("PaidBy").Where("group_id = ?", groupID).Find(&expenses).Error
	if err != nil {
		return nil, err
	}
	return expenses, nil
}
