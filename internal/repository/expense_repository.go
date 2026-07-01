package repository

import (
	"context"

	"spliteasy/internal/domain"

	"gorm.io/gorm"
)

type ExpenseRepository interface {
	CreateWithSplits(ctx context.Context, expense *domain.Expense, splits []domain.ExpenseSplit, items []domain.ExpenseItem) error
	// UpdateWithSplits replaces an expense's fields, splits, and items
	// entirely — the old splits/items (and item-member join rows) are
	// deleted and the new ones inserted, all in one transaction.
	UpdateWithSplits(ctx context.Context, expense *domain.Expense, splits []domain.ExpenseSplit, items []domain.ExpenseItem) error
	GetByID(ctx context.Context, id uint) (*domain.Expense, error)
	GetByGroupID(ctx context.Context, groupID uint) ([]domain.Expense, error)
	// Delete soft-deletes the expense (sets deleted_at); it's excluded from
	// every normal query afterward but the row itself is kept.
	Delete(ctx context.Context, id uint) error
}

type expenseRepository struct {
	db *gorm.DB
}

func NewExpenseRepository(db *gorm.DB) ExpenseRepository {
	return &expenseRepository{db}
}

// insertItems inserts line items (if any) and their member assignments.
// Omits the Users association on Create so gorm doesn't try to upsert user
// rows; the join rows are inserted explicitly instead.
func insertItems(tx *gorm.DB, expenseID uint, items []domain.ExpenseItem) error {
	if len(items) == 0 {
		return nil
	}
	for i := range items {
		items[i].ID = 0
		items[i].ExpenseID = expenseID
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
	return nil
}

// CreateWithSplits creates an expense, its splits, and (optionally) its line
// items with their per-member assignments, all in a single transaction.
func (r *expenseRepository) CreateWithSplits(ctx context.Context, expense *domain.Expense, splits []domain.ExpenseSplit, items []domain.ExpenseItem) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(expense).Error; err != nil {
			return err
		}

		for i := range splits {
			splits[i].ExpenseID = expense.ID
		}
		if err := tx.Create(&splits).Error; err != nil {
			return err
		}

		return insertItems(tx, expense.ID, items)
	})
}

// UpdateWithSplits updates the expense's own fields (paid_by_id, description,
// amount) and replaces its splits/items wholesale — editing a split method
// can change how many rows there are and for whom, so patching individual
// rows in place isn't meaningful; the old set is deleted and the new set
// inserted instead.
func (r *expenseRepository) UpdateWithSplits(ctx context.Context, expense *domain.Expense, splits []domain.ExpenseSplit, items []domain.ExpenseItem) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// A map (rather than passing the struct to Updates) so a zero-value
		// field — an empty description, in principle — is still written
		// instead of silently skipped.
		if err := tx.Model(&domain.Expense{}).Where("id = ?", expense.ID).Updates(map[string]interface{}{
			"paid_by_id":  expense.PaidByID,
			"description": expense.Description,
			"amount":      expense.Amount,
		}).Error; err != nil {
			return err
		}

		if err := tx.Where("expense_id = ?", expense.ID).Delete(&domain.ExpenseSplit{}).Error; err != nil {
			return err
		}
		for i := range splits {
			splits[i].ID = 0
			splits[i].ExpenseID = expense.ID
		}
		if len(splits) > 0 {
			if err := tx.Create(&splits).Error; err != nil {
				return err
			}
		}

		var oldItemIDs []uint
		if err := tx.Model(&domain.ExpenseItem{}).Where("expense_id = ?", expense.ID).Pluck("id", &oldItemIDs).Error; err != nil {
			return err
		}
		if len(oldItemIDs) > 0 {
			if err := tx.Exec("DELETE FROM expense_item_users WHERE expense_item_id IN (?)", oldItemIDs).Error; err != nil {
				return err
			}
			if err := tx.Where("expense_id = ?", expense.ID).Delete(&domain.ExpenseItem{}).Error; err != nil {
				return err
			}
		}

		return insertItems(tx, expense.ID, items)
	})
}

func (r *expenseRepository) GetByID(ctx context.Context, id uint) (*domain.Expense, error) {
	var expense domain.Expense
	err := r.db.WithContext(ctx).
		Preload("Splits").
		Preload("PaidBy").
		Preload("Items").
		Preload("Items.Users").
		First(&expense, id).Error
	if err != nil {
		return nil, err
	}
	return &expense, nil
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

func (r *expenseRepository) Delete(ctx context.Context, id uint) error {
	return r.db.WithContext(ctx).Delete(&domain.Expense{}, id).Error
}
