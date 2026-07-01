package service

import "spliteasy/internal/domain"

// isMember reports whether userID belongs to group.Members. Shared across
// service implementations that need to check membership against an
// already-loaded group (group_service and balance_service) — keep any new
// membership check here instead of re-implementing it locally.
func isMember(group *domain.Group, userID uint) bool {
	for _, member := range group.Members {
		if member.ID == userID {
			return true
		}
	}
	return false
}
