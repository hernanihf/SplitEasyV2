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
