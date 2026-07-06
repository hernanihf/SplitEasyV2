package repository

import (
	"context"

	"spliteasy/internal/domain"

	"gorm.io/gorm"
)

type GroupRepository interface {
	Create(ctx context.Context, group *domain.Group) error
	GetByID(ctx context.Context, id uint) (*domain.Group, error)
	GetByUserID(ctx context.Context, userID uint) ([]domain.Group, error)
	GetByInviteToken(ctx context.Context, token string) (*domain.Group, error)
	AddMember(ctx context.Context, groupID, userID uint) error
	SetInviteTokenIfEmpty(ctx context.Context, groupID uint, token string) error
	// UpdateNameAndEmoji updates only the fields that are non-nil, so a
	// caller can patch the name, the emoji, or both in one call without
	// clobbering the field it left untouched.
	UpdateNameAndEmoji(ctx context.Context, groupID uint, name, emoji *string) error
	// GetExpenseReceiptImagePaths returns the storage path of every receipt
	// image attached to any expense in the group (including already
	// soft-deleted expenses) — collected before Delete so the caller can
	// clean up Supabase Storage afterward.
	GetExpenseReceiptImagePaths(ctx context.Context, groupID uint) ([]string, error)
	// Delete permanently removes the group and everything under it — every
	// expense (with its splits/items), every settlement, and every comment
	// on either, regardless of prior soft-delete state. None of the
	// relevant foreign keys cascade today, so this deletes in dependency
	// order inside one transaction. Irreversible.
	Delete(ctx context.Context, groupID uint) error
}

type groupRepository struct {
	db *gorm.DB
}

func NewGroupRepository(db *gorm.DB) GroupRepository {
	return &groupRepository{db}
}

func (r *groupRepository) Create(ctx context.Context, group *domain.Group) error {
	return r.db.WithContext(ctx).Create(group).Error
}

func (r *groupRepository) GetByID(ctx context.Context, id uint) (*domain.Group, error) {
	var group domain.Group
	err := r.db.WithContext(ctx).Preload("Members").First(&group, id).Error
	if err != nil {
		return nil, err
	}
	return &group, nil
}

func (r *groupRepository) GetByUserID(ctx context.Context, userID uint) ([]domain.Group, error) {
	var groups []domain.Group
	err := r.db.WithContext(ctx).Preload("Members").
		Joins("JOIN group_users ON group_users.group_id = groups.id").
		Where("group_users.user_id = ?", userID).
		Find(&groups).Error
	if err != nil {
		return nil, err
	}
	return groups, nil
}

func (r *groupRepository) GetByInviteToken(ctx context.Context, token string) (*domain.Group, error) {
	var group domain.Group
	err := r.db.WithContext(ctx).Preload("Members").Where("invite_token = ?", token).First(&group).Error
	if err != nil {
		return nil, err
	}
	return &group, nil
}

// AddMember inserts a membership row, ignoring the insert if the user is
// already a member (idempotent).
func (r *groupRepository) AddMember(ctx context.Context, groupID, userID uint) error {
	return r.db.WithContext(ctx).Exec(
		"INSERT INTO group_users (group_id, user_id) VALUES (?, ?) ON CONFLICT DO NOTHING",
		groupID, userID,
	).Error
}

// SetInviteTokenIfEmpty atomically sets the token only when the group has none,
// so concurrent "generate a token" requests can't clobber each other — the
// first writer wins and the others' conditional update is a no-op.
func (r *groupRepository) SetInviteTokenIfEmpty(ctx context.Context, groupID uint, token string) error {
	return r.db.WithContext(ctx).Model(&domain.Group{}).
		Where("id = ? AND (invite_token IS NULL OR invite_token = '')", groupID).
		Update("invite_token", token).Error
}

func (r *groupRepository) UpdateNameAndEmoji(ctx context.Context, groupID uint, name, emoji *string) error {
	updates := map[string]interface{}{}
	if name != nil {
		updates["name"] = *name
	}
	if emoji != nil {
		updates["emoji"] = *emoji
	}
	if len(updates) == 0 {
		return nil
	}
	return r.db.WithContext(ctx).Model(&domain.Group{}).Where("id = ?", groupID).Updates(updates).Error
}

func (r *groupRepository) GetExpenseReceiptImagePaths(ctx context.Context, groupID uint) ([]string, error) {
	var paths []string
	err := r.db.WithContext(ctx).Unscoped().Model(&domain.Expense{}).
		Where("group_id = ? AND receipt_image_path IS NOT NULL", groupID).
		Pluck("receipt_image_path", &paths).Error
	if err != nil {
		return nil, err
	}
	return paths, nil
}

// Delete deletes in FK-dependency order (see the interface doc comment) using
// raw SQL — a plain Exec, unlike GORM's model-based Delete, doesn't apply the
// soft-delete default scope, so already-deleted rows are purged along with
// everything else instead of being left behind forever.
func (r *groupRepository) Delete(ctx context.Context, groupID uint) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		stmts := []struct {
			sql  string
			args []interface{}
		}{
			{
				`DELETE FROM comments WHERE expense_id IN (SELECT id FROM expenses WHERE group_id = ?)
				 OR settlement_id IN (SELECT id FROM settlements WHERE group_id = ?)`,
				[]interface{}{groupID, groupID},
			},
			{
				`DELETE FROM expense_item_users WHERE expense_item_id IN (
					SELECT id FROM expense_items WHERE expense_id IN (SELECT id FROM expenses WHERE group_id = ?)
				 )`,
				[]interface{}{groupID},
			},
			{
				`DELETE FROM expense_items WHERE expense_id IN (SELECT id FROM expenses WHERE group_id = ?)`,
				[]interface{}{groupID},
			},
			{
				`DELETE FROM expense_splits WHERE expense_id IN (SELECT id FROM expenses WHERE group_id = ?)`,
				[]interface{}{groupID},
			},
			{`DELETE FROM expenses WHERE group_id = ?`, []interface{}{groupID}},
			{`DELETE FROM settlements WHERE group_id = ?`, []interface{}{groupID}},
			{`DELETE FROM group_users WHERE group_id = ?`, []interface{}{groupID}},
			{`DELETE FROM groups WHERE id = ?`, []interface{}{groupID}},
		}
		for _, stmt := range stmts {
			if err := tx.Exec(stmt.sql, stmt.args...).Error; err != nil {
				return err
			}
		}
		return nil
	})
}
