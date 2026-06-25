package domain

import (
	"time"
)

type Expense struct {
	ID          uint      `gorm:"primaryKey" json:"id"`
	GroupID     uint      `gorm:"not null" json:"group_id"`
	PaidByID    uint      `gorm:"not null" json:"paid_by_id"`
	Description string    `gorm:"not null" json:"description"`
	Amount      float64   `gorm:"type:numeric(10,2);not null" json:"amount"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`

	// Relationships
	PaidBy User           `gorm:"foreignKey:PaidByID" json:"paid_by"`
	Group  Group          `gorm:"foreignKey:GroupID" json:"-"`
	Splits []ExpenseSplit `json:"splits"`
}
