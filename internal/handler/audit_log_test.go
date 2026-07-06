package handler

import (
	"context"
)

type auditRecordCall struct {
	groupID, actorID, entityID uint
	action, entityType, detail string
}

type fakeAuditService struct {
	records []auditRecordCall
}

func (f *fakeAuditService) Record(_ context.Context, groupID, actorID uint, action, entityType string, entityID uint, detail string) {
	f.records = append(f.records, auditRecordCall{groupID, actorID, entityID, action, entityType, detail})
}
