package domain

import (
	"time"
)

type User struct {
	ID        uint   `gorm:"primaryKey" json:"id"`
	Name      string `gorm:"not null" json:"name"`
	Email     string `gorm:"unique;not null" json:"email"`
	AvatarURL string `json:"avatar_url"`
	// PushEnabled is the user's own preference, checked before sending any
	// push notification — separate from whether they have any subscribed
	// devices (PushSubscription rows).
	PushEnabled bool      `gorm:"not null;default:true" json:"push_enabled"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}
