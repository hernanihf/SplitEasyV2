package domain

import (
	"time"
)

type Expense struct {
	ID          uint      `gorm:"primaryKey" json:"id"`
	GroupID     uint      `gorm:"not null" json:"group_id"`
	PaidByID    uint      `gorm:"not null" json:"paid_by_id"`
	Description string    `gorm:"not null" json:"description"`
	Amount      int64     `gorm:"not null" json:"amount"` // cents
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`

	// Relationships
	PaidBy User           `gorm:"foreignKey:PaidByID" json:"paid_by"`
	Group  Group          `gorm:"foreignKey:GroupID" json:"-"`
	Splits []ExpenseSplit `json:"splits"`
}
