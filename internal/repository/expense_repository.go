package repository

import (
	"context"

	"spliteasy/internal/domain"

	"gorm.io/gorm"
)

type ExpenseRepository interface {
	CreateWithSplits(ctx context.Context, expense *domain.Expense, splits []domain.ExpenseSplit, items []domain.ExpenseItem) error
	GetByGroupID(ctx context.Context, groupID uint) ([]domain.Expense, error)
}

type expenseRepository struct {
	db *gorm.DB
}

func NewExpenseRepository(db *gorm.DB) ExpenseRepository {
	return &expenseRepository{db}
}

// CreateWithSplits creates an expense, its splits, and (optionally) its line
// items with their per-member assignments, all in a single transaction.
func (r *expenseRepository) CreateWithSplits(ctx context.Context, expense *domain.Expense, splits []domain.ExpenseSplit, items []domain.ExpenseItem) error {
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

		// Insert line items (if any) and their member assignments. Omit the
		// Users association on Create so gorm doesn't try to upsert user rows;
		// the join rows are inserted explicitly instead.
		if len(items) > 0 {
			for i := range items {
				items[i].ExpenseID = expense.ID
			}
			if err := tx.Omit("Users").Create(&items).Error; err != nil {
				return err
			}
			for _, item := range items {
				for _, u := range item.Users {
					if err := tx.Exec(
						"INSERT INTO expense_item_users (expense_item_id, user_id) VALUES (?, ?) ON CONFLICT DO NOTHING",
						item.ID, u.ID,
					).Error; err != nil {
						return err
					}
				}
			}
		}

		return nil
	})
}

func (r *expenseRepository) GetByGroupID(ctx context.Context, groupID uint) ([]domain.Expense, error) {
	var expenses []domain.Expense
	err := r.db.WithContext(ctx).
		Preload("Splits").
		Preload("PaidBy").
		Preload("Items").
		Preload("Items.Users").
		Where("group_id = ?", groupID).
		Find(&expenses).Error
	if err != nil {
		return nil, err
	}
	return expenses, nil
}
