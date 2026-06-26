package domain

// ExpenseItem is a single line item of an itemized expense (e.g. one dish on a
// restaurant bill). It is assigned to one or more members, who share its cost
// equally. Items are stored for display/audit; the actual debt is captured by
// the expense's Splits.
type ExpenseItem struct {
	ID          uint   `gorm:"primaryKey" json:"id"`
	ExpenseID   uint   `gorm:"not null" json:"expense_id"`
	Description string `gorm:"not null" json:"description"`
	Amount      int64  `gorm:"not null" json:"amount"` // cents

	// Members this item is split among (equally).
	Users []User `gorm:"many2many:expense_item_users;" json:"users"`
}
