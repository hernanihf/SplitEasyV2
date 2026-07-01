package domain

import (
	"time"

	"gorm.io/gorm"
)

// Comment is a free-text note posted by a group member on either an expense
// or a settlement (exactly one of ExpenseID/SettlementID is set, enforced by
// a DB check constraint), for discussing or clarifying it inline.
type Comment struct {
	ID           uint      `gorm:"primaryKey" json:"id"`
	ExpenseID    *uint     `json:"expense_id,omitempty"`
	SettlementID *uint     `json:"settlement_id,omitempty"`
	UserID       uint      `gorm:"not null" json:"user_id"`
	Body         string    `gorm:"not null" json:"body"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`

	// A deleted comment is excluded from every normal query by GORM's
	// default scope, but the row is kept for dispute resolution — same
	// pattern as expenses and settlements.
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`

	User User `gorm:"foreignKey:UserID" json:"user"`
}
