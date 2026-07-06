package service

import (
	"context"
	"errors"
	"testing"

	"spliteasy/internal/domain"
)

type fakeAuditLogRepository struct {
	created   []*domain.AuditLog
	createErr error
}

func (f *fakeAuditLogRepository) Create(_ context.Context, entry *domain.AuditLog) error {
	if f.createErr != nil {
		return f.createErr
	}
	f.created = append(f.created, entry)
	return nil
}

func TestAuditService_RecordPersistsEntry(t *testing.T) {
	repo := &fakeAuditLogRepository{}
	svc := NewAuditService(repo)

	svc.Record(context.Background(), 1, 2, domain.AuditActionExpenseDeleted, domain.AuditEntityExpense, 5, "Dinner")

	if len(repo.created) != 1 {
		t.Fatalf("expected 1 audit entry to be created, got %d", len(repo.created))
	}
	entry := repo.created[0]
	if entry.GroupID != 1 || entry.ActorID != 2 || entry.Action != domain.AuditActionExpenseDeleted ||
		entry.EntityType != domain.AuditEntityExpense || entry.EntityID != 5 || entry.Detail != "Dinner" {
		t.Errorf("unexpected audit entry: %+v", entry)
	}
}

func TestAuditService_RecordSwallowsRepositoryError(t *testing.T) {
	// A broken audit write must never surface to (or block) the caller — the
	// action being audited already succeeded by the time Record is called.
	repo := &fakeAuditLogRepository{createErr: errors.New("db is down")}
	svc := NewAuditService(repo)

	svc.Record(context.Background(), 1, 2, domain.AuditActionGroupDeleted, domain.AuditEntityGroup, 1, "Trip to BA")
	// No panic and no return value to check — reaching this line is the test.
}
