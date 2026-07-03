package repository

import (
	"context"

	"spliteasy/internal/domain"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type PushSubscriptionRepository interface {
	// Create upserts on (user_id, endpoint) — re-subscribing the same
	// browser (e.g. after a permission re-grant) updates the existing row's
	// keys instead of creating a duplicate.
	Create(ctx context.Context, sub *domain.PushSubscription) error
	ListByUserIDs(ctx context.Context, userIDs []uint) ([]domain.PushSubscription, error)
	DeleteByEndpoint(ctx context.Context, userID uint, endpoint string) error
	// DeleteByEndpointGlobal removes a subscription by endpoint alone,
	// regardless of owner — used when the push service reports an endpoint
	// as gone (404/410), since that cleanup doesn't have a caller identity.
	DeleteByEndpointGlobal(ctx context.Context, endpoint string) error
	CountByUserID(ctx context.Context, userID uint) (int64, error)
}

type pushSubscriptionRepository struct {
	db *gorm.DB
}

func NewPushSubscriptionRepository(db *gorm.DB) PushSubscriptionRepository {
	return &pushSubscriptionRepository{db}
}

func (r *pushSubscriptionRepository) Create(ctx context.Context, sub *domain.PushSubscription) error {
	return r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "user_id"}, {Name: "endpoint"}},
		DoUpdates: clause.AssignmentColumns([]string{"p256dh", "auth"}),
	}).Create(sub).Error
}

func (r *pushSubscriptionRepository) ListByUserIDs(ctx context.Context, userIDs []uint) ([]domain.PushSubscription, error) {
	var subs []domain.PushSubscription
	err := r.db.WithContext(ctx).Where("user_id IN ?", userIDs).Find(&subs).Error
	if err != nil {
		return nil, err
	}
	return subs, nil
}

func (r *pushSubscriptionRepository) DeleteByEndpoint(ctx context.Context, userID uint, endpoint string) error {
	return r.db.WithContext(ctx).
		Where("user_id = ? AND endpoint = ?", userID, endpoint).
		Delete(&domain.PushSubscription{}).Error
}

func (r *pushSubscriptionRepository) DeleteByEndpointGlobal(ctx context.Context, endpoint string) error {
	return r.db.WithContext(ctx).Where("endpoint = ?", endpoint).Delete(&domain.PushSubscription{}).Error
}

func (r *pushSubscriptionRepository) CountByUserID(ctx context.Context, userID uint) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&domain.PushSubscription{}).Where("user_id = ?", userID).Count(&count).Error
	return count, err
}
