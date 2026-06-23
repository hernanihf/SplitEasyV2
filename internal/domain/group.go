package domain

import (
	"time"
)

type Group struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	Name      string    `gorm:"not null" json:"name"`
	CreatedBy uint      `json:"created_by"`
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
