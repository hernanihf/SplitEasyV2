package domain

import (
	"time"

	"gorm.io/gorm"
)

type Expense struct {
	ID          uint      `gorm:"primaryKey" json:"id"`
	GroupID     uint      `gorm:"not null" json:"group_id"`
	PaidByID    uint      `gorm:"not null" json:"paid_by_id"`
	Description string    `gorm:"not null" json:"description"`
	Amount      int64     `gorm:"not null" json:"amount"` // cents
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`

	// A deleted expense is excluded from every normal query (including
	// balance calculations) by GORM's default scope, but the row — and the
	// fact that it existed — is kept for dispute resolution.
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`

	// Relationships
	PaidBy User           `gorm:"foreignKey:PaidByID" json:"paid_by"`
	Group  Group          `gorm:"foreignKey:GroupID" json:"-"`
	Splits []ExpenseSplit `json:"splits"`
	Items  []ExpenseItem  `json:"items,omitempty"`
}
