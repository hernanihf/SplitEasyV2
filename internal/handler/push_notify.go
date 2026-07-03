package handler

import (
	"context"
	"fmt"
	"log/slog"

	"spliteasy/internal/service"
)

// notifyGroupMembersAsync fires a push notification to a group's members in
// the background, after the triggering request's response has already been
// written — so a slow or failing push send never adds latency or an error
// to the action that caused it. Uses context.Background() rather than the
// request's context, which is canceled as soon as the handler returns.
func notifyGroupMembersAsync(pushService service.PushService, groupID, actorID uint, bodyFor func(actorName string) string) {
	if pushService == nil {
		return
	}
	go func() {
		data := map[string]string{"url": fmt.Sprintf("/groups/%d", groupID)}
		if err := pushService.NotifyGroupMembers(context.Background(), groupID, actorID, bodyFor, data); err != nil {
			slog.Error("failed to send push notification", "error", err, "group_id", groupID) //nolint:gosec // G706: groupID is a uint, can't inject log lines
		}
	}()
}
