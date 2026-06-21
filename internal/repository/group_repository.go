package repository

import (
	"spliteasy/internal/domain"

	"gorm.io/gorm"
)

type GroupRepository interface {
	Create(group *domain.Group) error
	GetByID(id uint) (*domain.Group, error)
	GetByUserID(userID uint) ([]domain.Group, error)
}

type groupRepository struct {
	db *gorm.DB
}

func NewGroupRepository(db *gorm.DB) GroupRepository {
	return &groupRepository{db}
}

func (r *groupRepository) Create(group *domain.Group) error {
	return r.db.Create(group).Error
}

func (r *groupRepository) GetByID(id uint) (*domain.Group, error) {
	var group domain.Group
	err := r.db.Preload("Members").First(&group, id).Error
	if err != nil {
		return nil, err
	}
	return &group, nil
}

func (r *groupRepository) GetByUserID(userID uint) ([]domain.Group, error) {
	var groups []domain.Group
	err := r.db.Preload("Members").
		Joins("JOIN group_users ON group_users.group_id = groups.id").
		Where("group_users.user_id = ?", userID).
		Find(&groups).Error
	if err != nil {
		return nil, err
	}
	return groups, nil
}
