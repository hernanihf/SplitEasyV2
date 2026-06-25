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
