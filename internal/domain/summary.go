package domain

import "time"

// OverallBalance is the authenticated user's aggregated balance across all of
// their groups. Net is owed minus owe (positive means they are owed money).
type OverallBalance struct {
	Net  int64 `json:"net"`  // cents
	Owed int64 `json:"owed"` // cents
	Owe  int64 `json:"owe"`  // cents
}

// GroupSummary is a group as shown on the home screen, with the current user's
// net balance in it.
type GroupSummary struct {
	ID           uint   `json:"id"`
	Name         string `json:"name"`
	Emoji        string `json:"emoji"`
	MembersCount int    `json:"members_count"`
	YourBalance  int64  `json:"your_balance"` // cents
}

// HomeSummary powers the home screen in a single request.
type HomeSummary struct {
	Overall OverallBalance `json:"overall"`
	Groups  []GroupSummary `json:"groups"`
}

// ActivityEvent is a single entry in the cross-group activity feed.
type ActivityEvent struct {
	// ID is the underlying expense's or settlement's id (whichever Type
	// says this event is), for opening the same detail view group history
	// rows link to.
	ID         uint      `json:"id"`
	Type       string    `json:"type"` // "expense" | "settlement"
	GroupID    uint      `json:"group_id"`
	GroupName  string    `json:"group_name"`
	GroupEmoji string    `json:"group_emoji"`
	Title      string    `json:"title"`
	ActorID    uint      `json:"actor_id"`
	ActorName  string    `json:"actor_name"`
	Amount     int64     `json:"amount"`     // cents
	YourShare  int64     `json:"your_share"` // cents
	Date       time.Time `json:"date"`
}
