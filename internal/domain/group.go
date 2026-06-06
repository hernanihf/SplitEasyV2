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
	
	// Relationships
	Members  []User    `gorm:"many2many:group_users;" json:"members"`
	Expenses []Expense `json:"expenses,omitempty"`
}
