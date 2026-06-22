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
	MerchantName string        `json:"merchant_name"`
	Date         string        `json:"date"`
	TotalAmount  float64       `json:"total_amount"`
	Items        []ReceiptItem `json:"items"`
}
