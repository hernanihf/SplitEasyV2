package domain

import "time"

// OverallBalance is the authenticated user's aggregated balance across every
// group that shares its currency. Net is owed minus owe (positive means they
// are owed money). Groups in different currencies can't be summed into one
// number without a conversion rate, so the home screen gets one of these per
// currency instead of a single total.
type OverallBalance struct {
	Currency string `json:"currency"`
	Net      int64  `json:"net"`  // cents
	Owed     int64  `json:"owed"` // cents
	Owe      int64  `json:"owe"`  // cents
}

// GroupSummary is a group as shown on the home screen, with the current user's
// net balance in it.
type GroupSummary struct {
	ID           uint   `json:"id"`
	Name         string `json:"name"`
	Emoji        string `json:"emoji"`
	Currency     string `json:"currency"`
	MembersCount int    `json:"members_count"`
	YourBalance  int64  `json:"your_balance"` // cents
	// CreatedBy lets the frontend show a delete affordance only to the
	// group's creator.
	CreatedBy uint `json:"created_by"`
}

// HomeSummary powers the home screen in a single request.
type HomeSummary struct {
	OverallByCurrency []OverallBalance `json:"overall_by_currency"`
	Groups            []GroupSummary   `json:"groups"`
}

// ActivityEvent is a single entry in the cross-group activity feed.
type ActivityEvent struct {
	// ID is the underlying expense's or settlement's id (whichever Type/
	// ParentType says this event is about), for opening the same detail view
	// group history rows link to. For a "comment" event this is its parent
	// expense/settlement's id, not the comment's own id — several comments
	// on the same expense all point to that one detail view.
	ID         uint   `json:"id"`
	Type       string `json:"type"` // "expense" | "settlement" | "comment"
	GroupID    uint   `json:"group_id"`
	GroupName  string `json:"group_name"`
	GroupEmoji string `json:"group_emoji"`
	Currency   string `json:"currency"`
	// Title is the expense's description, the "X paid Y" line for a
	// settlement, or the comment's own text for a comment event.
	Title string `json:"title"`
	// Category is the expense's category slug (empty for settlements and
	// comments) so the feed can show the same icon the group history and
	// detail views do.
	Category  string    `json:"category,omitempty"`
	ActorID   uint      `json:"actor_id"`
	ActorName string    `json:"actor_name"`
	Amount    int64     `json:"amount"`     // cents — 0 for comments
	YourShare int64     `json:"your_share"` // cents — 0 for comments
	Date      time.Time `json:"date"`
	// Deleted (expenses only — a settlement can't be soft-deleted) means this
	// is a tombstone: still shown for transparency, but crossed out and
	// non-interactive in the feed, same as in the group's own history.
	Deleted       bool   `json:"deleted,omitempty"`
	DeletedByName string `json:"deleted_by_name,omitempty"`
	// ParentType and ParentTitle are set only for "comment" events: which
	// kind of thing (and its title/description) the comment was left on, so
	// the feed can render "commented on <ParentTitle>".
	ParentType  string `json:"parent_type,omitempty"`
	ParentTitle string `json:"parent_title,omitempty"`
}
