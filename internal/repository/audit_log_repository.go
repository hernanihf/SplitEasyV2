package repository

import (
	"context"

	"spliteasy/internal/domain"

	"gorm.io/gorm"
)

type AuditLogRepository interface {
	Create(ctx context.Context, entry *domain.AuditLog) error
}

type auditLogRepository struct {
	db *gorm.DB
}

func NewAuditLogRepository(db *gorm.DB) AuditLogRepository {
	return &auditLogRepository{db}
}

func (r *auditLogRepository) Create(ctx context.Context, entry *domain.AuditLog) error {
	return r.db.WithContext(ctx).Create(entry).Error
}
