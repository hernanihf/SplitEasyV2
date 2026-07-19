package repository

import (
	"context"
	"time"

	"spliteasy/internal/domain"

	"gorm.io/gorm"
)

type UserRepository interface {
	Create(ctx context.Context, user *domain.User) error
	GetByID(ctx context.Context, id uint) (*domain.User, error)
	GetByEmail(ctx context.Context, email string) (*domain.User, error)
	Update(ctx context.Context, user *domain.User) error
	// UpdatePushPreferences sets the master push switch and all three
	// per-category flags in one write — the frontend always sends its full
	// current state (see SetPushPreferenceRequest), so there's no need for
	// per-field partial updates.
	UpdatePushPreferences(ctx context.Context, userID uint, enabled, expenses, payments, comments bool) error
	// UpdateActivityLastSeenAt records when the user last viewed the
	// activity feed — the cutoff SummaryService.GetUnreadActivityCount uses.
	UpdateActivityLastSeenAt(ctx context.Context, userID uint, seenAt time.Time) error
}

type userRepository struct {
	db *gorm.DB
}

func NewUserRepository(db *gorm.DB) UserRepository {
	return &userRepository{db}
}

func (r *userRepository) Create(ctx context.Context, user *domain.User) error {
	return r.db.WithContext(ctx).Create(user).Error
}

func (r *userRepository) Update(ctx context.Context, user *domain.User) error {
	return r.db.WithContext(ctx).Save(user).Error
}

func (r *userRepository) GetByID(ctx context.Context, id uint) (*domain.User, error) {
	var user domain.User
	err := r.db.WithContext(ctx).First(&user, id).Error
	if err != nil {
		return nil, err
	}
	return &user, nil
}

func (r *userRepository) GetByEmail(ctx context.Context, email string) (*domain.User, error) {
	var user domain.User
	err := r.db.WithContext(ctx).Where("email = ?", email).First(&user).Error
	if err != nil {
		return nil, err
	}
	return &user, nil
}

func (r *userRepository) UpdatePushPreferences(ctx context.Context, userID uint, enabled, expenses, payments, comments bool) error {
	return r.db.WithContext(ctx).Model(&domain.User{}).Where("id = ?", userID).
		Updates(map[string]interface{}{
			"push_enabled":          enabled,
			"push_expenses_enabled": expenses,
			"push_payments_enabled": payments,
			"push_comments_enabled": comments,
		}).Error
}

func (r *userRepository) UpdateActivityLastSeenAt(ctx context.Context, userID uint, seenAt time.Time) error {
	return r.db.WithContext(ctx).Model(&domain.User{}).Where("id = ?", userID).
		Update("activity_last_seen_at", seenAt).Error
}
