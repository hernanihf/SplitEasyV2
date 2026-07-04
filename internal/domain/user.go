package domain

import (
	"time"
)

type User struct {
	ID        uint   `gorm:"primaryKey" json:"id"`
	Name      string `gorm:"not null" json:"name"`
	Email     string `gorm:"unique;not null" json:"email"`
	AvatarURL string `json:"avatar_url"`
	// PushEnabled is the master switch — checked before sending any push
	// notification, separate from whether they have any subscribed devices
	// (PushSubscription rows). The three category flags below only matter
	// while this is also true; they let a user keep push on but tune which
	// kinds of group activity actually notify them.
	PushEnabled         bool `gorm:"not null;default:true" json:"push_enabled"`
	PushExpensesEnabled bool `gorm:"not null;default:true" json:"push_expenses_enabled"`
	PushPaymentsEnabled bool `gorm:"not null;default:true" json:"push_payments_enabled"`
	PushCommentsEnabled bool `gorm:"not null;default:true" json:"push_comments_enabled"`
	// DefaultCurrency seeds the currency field when this user creates a new
	// group — best-effort guessed from their Google account locale at
	// login (see auth_service.go's currencyFromLocale), never validated
	// against anything the user does afterward.
	DefaultCurrency string    `gorm:"not null;default:'ARS'" json:"default_currency"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}
