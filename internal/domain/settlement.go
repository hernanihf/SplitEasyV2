package domain

import (
	"time"

	"gorm.io/gorm"
)

// Settlement records a payment made to settle (part of) a debt between two
// members of a group. Balances are always recalculated from Expenses plus
// Settlements — a Settlement is never edited, only created or deleted.
type Settlement struct {
	ID         uint      `gorm:"primaryKey" json:"id"`
	GroupID    uint      `gorm:"not null" json:"group_id"`
	FromUserID uint      `gorm:"not null" json:"from_user_id"`
	ToUserID   uint      `gorm:"not null" json:"to_user_id"`
	Amount     int64     `gorm:"not null" json:"amount"` // cents
	CreatedAt  time.Time `json:"created_at"`

	// A deleted settlement is excluded from every normal query (including
	// balance calculations) by GORM's default scope, but the row is kept
	// for dispute resolution.
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`

	// Relationships
	FromUser User `gorm:"foreignKey:FromUserID" json:"from_user"`
	ToUser   User `gorm:"foreignKey:ToUserID" json:"to_user"`
}
