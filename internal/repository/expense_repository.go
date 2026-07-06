package repository

import (
	"context"
	"time"

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
	// GetByGroupIDIncludingDeleted is like GetByGroupID but also returns
	// soft-deleted expenses (with DeletedBy preloaded), for the group's
	// history view — which shows a deleted expense struck through instead of
	// removing it outright. Balances and summaries must keep using
	// GetByGroupID; this is only for that one display case.
	GetByGroupIDIncludingDeleted(ctx context.Context, groupID uint) ([]domain.Expense, error)
	// Delete soft-deletes the expense (sets deleted_at and deleted_by_id);
	// it's excluded from every normal query afterward but the row itself is
	// kept.
	Delete(ctx context.Context, id, deletedByID uint) error
	// GetOldSoftDeletedReceiptImagePaths returns the receipt image path of
	// every expense soft-deleted before cutoff — collected before
	// PurgeOldSoftDeleted so the caller can clean up storage after the
	// hard-delete succeeds.
	GetOldSoftDeletedReceiptImagePaths(ctx context.Context, cutoff time.Time) ([]string, error)
	// PurgeOldSoftDeleted permanently deletes every expense (and its
	// comments/items/splits) soft-deleted before cutoff, regardless of how
	// long ago — the retention window is the caller's decision, this just
	// executes it. Returns how many expenses were purged. Irreversible.
	PurgeOldSoftDeleted(ctx context.Context, cutoff time.Time) (int64, error)
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
			"paid_by_id":         expense.PaidByID,
			"description":        expense.Description,
			"category":           expense.Category,
			"amount":             expense.Amount,
			"receipt_image_path": expense.ReceiptImagePath,
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

func (r *expenseRepository) GetByGroupIDIncludingDeleted(ctx context.Context, groupID uint) ([]domain.Expense, error) {
	var expenses []domain.Expense
	err := r.db.WithContext(ctx).
		Unscoped().
		Preload("Splits").
		Preload("PaidBy").
		Preload("DeletedBy").
		Preload("Items").
		Preload("Items.Users").
		Where("group_id = ?", groupID).
		Find(&expenses).Error
	if err != nil {
		return nil, err
	}
	return expenses, nil
}

// Delete sets deleted_at and deleted_by_id in one statement instead of
// GORM's own soft-delete (which only knows about deleted_at) — deleted_at is
// still the column every other query's default scope filters on, so this
// has the same soft-delete effect while also recording who did it.
func (r *expenseRepository) Delete(ctx context.Context, id, deletedByID uint) error {
	return r.db.WithContext(ctx).Model(&domain.Expense{}).Where("id = ?", id).Updates(map[string]interface{}{
		"deleted_at":    time.Now(),
		"deleted_by_id": deletedByID,
	}).Error
}

func (r *expenseRepository) GetOldSoftDeletedReceiptImagePaths(ctx context.Context, cutoff time.Time) ([]string, error) {
	var paths []string
	err := r.db.WithContext(ctx).Unscoped().Model(&domain.Expense{}).
		Where("deleted_at IS NOT NULL AND deleted_at < ? AND receipt_image_path IS NOT NULL", cutoff).
		Pluck("receipt_image_path", &paths).Error
	if err != nil {
		return nil, err
	}
	return paths, nil
}

// PurgeOldSoftDeleted deletes in FK-dependency order (same as the group
// cascade in group_repository.go) using raw SQL scoped by expense_id instead
// of group_id — a plain Exec doesn't apply GORM's soft-delete default scope,
// which is exactly what's needed here to actually reach already-deleted rows.
func (r *expenseRepository) PurgeOldSoftDeleted(ctx context.Context, cutoff time.Time) (int64, error) {
	var purged int64
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		const oldExpenseIDs = `SELECT id FROM expenses WHERE deleted_at IS NOT NULL AND deleted_at < ?`
		stmts := []struct {
			sql  string
			args []interface{}
		}{
			{
				`DELETE FROM comments WHERE expense_id IN (` + oldExpenseIDs + `)`,
				[]interface{}{cutoff},
			},
			{
				`DELETE FROM expense_item_users WHERE expense_item_id IN (
					SELECT id FROM expense_items WHERE expense_id IN (` + oldExpenseIDs + `)
				 )`,
				[]interface{}{cutoff},
			},
			{
				`DELETE FROM expense_items WHERE expense_id IN (` + oldExpenseIDs + `)`,
				[]interface{}{cutoff},
			},
			{
				`DELETE FROM expense_splits WHERE expense_id IN (` + oldExpenseIDs + `)`,
				[]interface{}{cutoff},
			},
		}
		for _, stmt := range stmts {
			if err := tx.Exec(stmt.sql, stmt.args...).Error; err != nil {
				return err
			}
		}

		result := tx.Exec(`DELETE FROM expenses WHERE deleted_at IS NOT NULL AND deleted_at < ?`, cutoff)
		if result.Error != nil {
			return result.Error
		}
		purged = result.RowsAffected
		return nil
	})
	return purged, err
}
