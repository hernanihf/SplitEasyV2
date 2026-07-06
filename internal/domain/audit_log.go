package domain

import "time"

// Audit actions recorded by AuditService — deliberately scoped to
// financial/destructive mutations (expenses, settlements, group deletion)
// rather than every read or write, since those are the ones worth
// investigating ("who deleted this?", "who changed the amount?"). Internal
// only, for now: queried directly against the database, not exposed via API.
const (
	AuditActionExpenseCreated    = "expense.created"
	AuditActionExpenseUpdated    = "expense.updated"
	AuditActionExpenseDeleted    = "expense.deleted"
	AuditActionSettlementCreated = "settlement.created"
	AuditActionSettlementDeleted = "settlement.deleted"
	AuditActionGroupDeleted      = "group.deleted"
)

const (
	AuditEntityExpense    = "expense"
	AuditEntitySettlement = "settlement"
	AuditEntityGroup      = "group"
)

// AuditLog is an immutable record of who did what, to what, and when, for a
// financial or destructive action within a group.
//
// GroupID has no foreign key constraint — see the migration for why: the
// group it refers to may since have been permanently deleted.
type AuditLog struct {
	ID         uint   `gorm:"primaryKey"`
	GroupID    uint   `gorm:"not null"`
	ActorID    uint   `gorm:"not null"`
	Action     string `gorm:"not null"`
	EntityType string `gorm:"not null"`
	EntityID   uint   `gorm:"not null"`
	Detail     string
	CreatedAt  time.Time
}
