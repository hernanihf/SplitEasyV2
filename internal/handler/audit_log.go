package handler

import (
	"context"

	"spliteasy/internal/service"
)

// recordAudit writes an audit entry if auditService is configured — nil is a
// valid value (test doubles pass it, matching notifyGroupMembersAsync's
// pushService convention). Unlike push notifications, this runs synchronously
// on the request goroutine rather than firing async: losing an audit entry
// to a crash right after the response is sent would defeat its purpose.
func recordAudit(ctx context.Context, auditService service.AuditService, groupID, actorID uint, action, entityType string, entityID uint, detail string) {
	if auditService == nil {
		return
	}
	auditService.Record(ctx, groupID, actorID, action, entityType, entityID, detail)
}
