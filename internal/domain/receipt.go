package domain

// ReceiptItem is one line item extracted from a scanned receipt/ticket.
type ReceiptItem struct {
	Description string  `json:"description"`
	Price       float64 `json:"price"`
}

// ReceiptScan is the structured data extracted from a receipt photo. It is
// never persisted directly — the frontend uses it to prefill an expense,
// which is then created through the normal /expenses endpoint.
type ReceiptScan struct {
	MerchantName string  `json:"merchant_name"`
	Date         string  `json:"date"`
	TotalAmount  float64 `json:"total_amount"`
	// Category is the model's suggested expense category — always one of
	// ExpenseCategorySlugs (coerced to the default when the model returns
	// anything else). The user can still change it before saving.
	Category string        `json:"category"`
	Items    []ReceiptItem `json:"items"`
}
