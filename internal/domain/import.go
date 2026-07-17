package domain

import "time"

// ImportRow is one parsed line from an uploaded expense-history CSV (e.g. a
// Splitwise export), before it's turned into a real Expense. Category is
// already mapped to one of our slugs (or "other"); the payer and per-member
// splits aren't resolved yet because that depends on the CSV-column→member
// mapping the frontend collects after showing this preview — MemberNets
// carries each column's raw net cents (positive = they're owed/paid,
// negative = they owe) so ImportService.Import can resolve it once that
// mapping is known.
type ImportRow struct {
	Date        time.Time        `json:"date"`
	Description string           `json:"description"`
	Category    string           `json:"category"`
	AmountCents int64            `json:"amount_cents"`
	MemberNets  map[string]int64 `json:"member_nets"`
}

// ImportPreview is the parsed, not-yet-committed result of uploading a CSV —
// shown to the user so they can map each detected column to a real group
// member before anything is actually created. SkippedRows counts lines that
// couldn't be parsed as an expense (blank lines, a trailing "total balance"
// summary row, rows in a different currency than the group's).
type ImportPreview struct {
	MemberColumns []string    `json:"member_columns"`
	Rows          []ImportRow `json:"rows"`
	SkippedRows   int         `json:"skipped_rows"`
}
