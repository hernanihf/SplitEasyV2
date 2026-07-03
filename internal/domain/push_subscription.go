package domain

import "time"

// PushSubscription is one browser/device's Web Push registration for a user.
// A user can have several (one per browser/device); each is addressed
// independently when sending a notification, and removed individually if
// the push service reports it's gone stale (404/410).
type PushSubscription struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	UserID    uint      `gorm:"not null" json:"user_id"`
	Endpoint  string    `gorm:"not null" json:"endpoint"`
	P256dh    string    `gorm:"not null" json:"-"`
	Auth      string    `gorm:"not null" json:"-"`
	CreatedAt time.Time `json:"created_at"`
}
