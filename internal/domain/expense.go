package domain

import (
	"time"

	"gorm.io/gorm"
)

type Expense struct {
	ID          uint   `gorm:"primaryKey" json:"id"`
	GroupID     uint   `gorm:"not null" json:"group_id"`
	PaidByID    uint   `gorm:"not null" json:"paid_by_id"`
	Description string `gorm:"not null" json:"description"`
	// Category is one of ExpenseCategorySlugs; it drives the expense's icon
	// and grouping in the frontend.
	Category string `gorm:"not null;default:other" json:"category"`
	Amount   int64  `gorm:"not null" json:"amount"` // cents
	// ReceiptImagePath is the Supabase Storage object key for the scanned
	// receipt image, if any — an internal storage reference, never handed to
	// clients directly (json:"-"). ReceiptImageURL is a short-lived signed
	// URL computed at response time from that path; it's never persisted; it
	// would go stale if it were.
	ReceiptImagePath *string   `json:"-"`
	ReceiptImageURL  string    `gorm:"-" json:"receipt_image_url,omitempty"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`

	// A deleted expense is excluded from every normal query (including
	// balance calculations) by GORM's default scope, but the row — and the
	// fact that it existed — is kept for dispute resolution. It's exposed in
	// JSON (unlike most soft-deleted rows in this codebase) so the group's
	// history can show it struck through instead of just vanishing.
	DeletedAt gorm.DeletedAt `gorm:"index" json:"deleted_at" swaggertype:"string"`
	// DeletedByID is who ran the delete — only meaningful once DeletedAt is
	// set. No default scope excludes it, so it stays populated on the
	// tombstoned row for GetByGroupIDIncludingDeleted to preload.
	DeletedByID *uint `json:"-"`
	DeletedBy   *User `gorm:"foreignKey:DeletedByID" json:"deleted_by,omitempty"`

	// Relationships
	PaidBy User           `gorm:"foreignKey:PaidByID" json:"paid_by"`
	Group  Group          `gorm:"foreignKey:GroupID" json:"-"`
	Splits []ExpenseSplit `json:"splits"`
	Items  []ExpenseItem  `json:"items,omitempty"`
}
