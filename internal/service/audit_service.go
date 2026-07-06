package service

import (
	"context"
	"log/slog"

	"spliteasy/internal/domain"
	"spliteasy/internal/repository"
)

// AuditService records accountability events for financial/destructive
// actions — who did what, to what, when — within a group. Write-only by
// design: the trail is for internal investigation (querying audit_logs
// directly), not a feature exposed to the app's users.
type AuditService interface {
	// Record persists one audit entry. It never returns an error: a broken
	// audit write must not block or fail the action it's recording, so a
	// failure is logged server-side instead.
	Record(ctx context.Context, groupID, actorID uint, action, entityType string, entityID uint, detail string)
}

type auditService struct {
	repo repository.AuditLogRepository
}

func NewAuditService(repo repository.AuditLogRepository) AuditService {
	return &auditService{repo}
}

func (s *auditService) Record(ctx context.Context, groupID, actorID uint, action, entityType string, entityID uint, detail string) {
	entry := &domain.AuditLog{
		GroupID:    groupID,
		ActorID:    actorID,
		Action:     action,
		EntityType: entityType,
		EntityID:   entityID,
		Detail:     detail,
	}
	if err := s.repo.Create(ctx, entry); err != nil {
		slog.Error("failed to write audit log", "error", err, "action", action)
	}
}
