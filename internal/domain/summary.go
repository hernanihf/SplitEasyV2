package domain

import "time"

// OverallBalance is the authenticated user's aggregated balance across all of
// their groups. Net is owed minus owe (positive means they are owed money).
type OverallBalance struct {
	Net  float64 `json:"net"`
	Owed float64 `json:"owed"`
	Owe  float64 `json:"owe"`
}

// GroupSummary is a group as shown on the home screen, with the current user's
// net balance in it.
type GroupSummary struct {
	ID           uint    `json:"id"`
	Name         string  `json:"name"`
	Emoji        string  `json:"emoji"`
	MembersCount int     `json:"members_count"`
	YourBalance  float64 `json:"your_balance"`
}

// HomeSummary powers the home screen in a single request.
type HomeSummary struct {
	Overall OverallBalance `json:"overall"`
	Groups  []GroupSummary `json:"groups"`
}

// ActivityEvent is a single entry in the cross-group activity feed.
type ActivityEvent struct {
	Type       string    `json:"type"` // "expense" | "settlement"
	GroupID    uint      `json:"group_id"`
	GroupName  string    `json:"group_name"`
	GroupEmoji string    `json:"group_emoji"`
	Title      string    `json:"title"`
	ActorID    uint      `json:"actor_id"`
	ActorName  string    `json:"actor_name"`
	Amount     float64   `json:"amount"`
	YourShare  float64   `json:"your_share"`
	Date       time.Time `json:"date"`
}
