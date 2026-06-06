package domain

import (
	"time"
)

type ExpenseSplit struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	ExpenseID uint      `gorm:"not null" json:"expense_id"`
	UserID    uint      `gorm:"not null" json:"user_id"`
	Amount    float64   `gorm:"type:numeric(10,2);not null" json:"amount"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	// Relationships
	User User `json:"user"`
}
