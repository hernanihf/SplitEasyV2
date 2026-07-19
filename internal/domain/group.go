package domain

import (
	"time"
)

type Group struct {
	ID        uint   `gorm:"primaryKey" json:"id"`
	Name      string `gorm:"not null" json:"name"`
	Emoji     string `json:"emoji"`
	CreatedBy uint   `json:"created_by"`
	// Currency is the ISO 4217 code (one of CurrencyCodes) every expense and
	// settlement in this group is denominated in. Fixed at creation — there's
	// no conversion, so changing it later would silently misinterpret every
	// amount already recorded.
	Currency  string    `gorm:"not null;default:USD" json:"currency"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	// InviteToken is a random, unguessable string used to build share links.
	// It is never included in the default JSON payload — it is only returned
	// through the dedicated member-only invite endpoint.
	InviteToken string `gorm:"uniqueIndex" json:"-"`

	// Relationships
	Members  []User    `gorm:"many2many:group_users;" json:"members"`
	Expenses []Expense `json:"expenses,omitempty"`
}

// GroupPreview is the limited view of a group shown to someone who has an
// invite link but isn't a member yet — before they commit to joining. It
// deliberately excludes anything only a member should see: the full member
// list, the invite token itself, expenses, balances. Holding the invite
// token already lets you become a full member (see JoinGroup), so this can't
// leak more than joining would; it exists purely so joining is an informed
// choice instead of a blind POST. CreatedByName is the one identity it does
// surface — who to credit the invite to — since the group's invite link is
// shared, not personal, so the creator is the closest available answer to
// "who invited me" rather than the literal sender of this link.
type GroupPreview struct {
	Name          string `json:"name"`
	Emoji         string `json:"emoji"`
	Currency      string `json:"currency"`
	MemberCount   int    `json:"member_count"`
	CreatedByName string `json:"created_by_name"`
}
