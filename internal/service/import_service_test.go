package service

import (
	"context"
	"encoding/csv"
	"errors"
	"strings"
	"testing"
	"time"

	"spliteasy/internal/domain"
)

func newImportService(group *domain.Group, expenses []domain.Expense, settlements []domain.Settlement) (ImportService, *fakeGroupRepo, *fakeExpenseRepo, *fakeSettlementRepo) {
	groupRepo := &fakeGroupRepo{group: group}
	expenseRepo := &fakeExpenseRepo{expenses: expenses}
	settlementRepo := &fakeSettlementRepo{settlements: settlements}
	return NewImportService(groupRepo, expenseRepo, settlementRepo), groupRepo, expenseRepo, settlementRepo
}

const sampleCSV = `Fecha,Descripción,Categoría,Coste,Moneda,Cami Vita Carino,Hernán Iannello
2023-09-24,Helado,Alimentos,4000.00,ARS,2000.00,-2000.00
2023-09-24,Almuerzo San Vicente central,Restaurantes,15555.00,ARS,-7777.50,7777.50
2025-03-19,Aceites,General,22000.00,ARS,22000.00,-22000.00
2026-01-08,Nafta YPF,Gasolina,20000.00,ARS,-20000.00,20000.00

2026-07-16,Saldo total, , ,ARS,-212232.12,212232.12
`

func TestParsePreview_ParsesRealisticSplitwiseExport(t *testing.T) {
	svc, _, _, _ := newImportService(&domain.Group{ID: 1, Currency: "ARS"}, nil, nil)

	preview, err := svc.ParsePreview(context.Background(), 1, strings.NewReader(sampleCSV))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if want := []string{"Cami Vita Carino", "Hernán Iannello"}; preview.MemberColumns[0] != want[0] || preview.MemberColumns[1] != want[1] {
		t.Errorf("expected member columns %v, got %v", want, preview.MemberColumns)
	}
	// 4 real expense rows; Go's csv.Reader already drops blank lines on its
	// own (never becomes a record at all), so only the trailing "Saldo
	// total" summary row (empty cost) shows up as skipped.
	if len(preview.Rows) != 4 {
		t.Fatalf("expected 4 parsed rows, got %d: %+v", len(preview.Rows), preview.Rows)
	}
	if preview.SkippedRows != 1 {
		t.Errorf("expected 1 skipped row (the summary row), got %d", preview.SkippedRows)
	}

	first := preview.Rows[0]
	if first.Description != "Helado" || first.Category != "groceries" || first.AmountCents != 400000 {
		t.Errorf("unexpected first row: %+v", first)
	}
	if first.Date.Format("2006-01-02") != "2023-09-24" {
		t.Errorf("expected date 2023-09-24, got %v", first.Date)
	}
	if first.MemberNets["Cami Vita Carino"] != 200000 || first.MemberNets["Hernán Iannello"] != -200000 {
		t.Errorf("unexpected member nets: %+v", first.MemberNets)
	}
}

func TestParsePreview_MapsGeneralCategoryToOther(t *testing.T) {
	svc, _, _, _ := newImportService(&domain.Group{ID: 1, Currency: "ARS"}, nil, nil)

	preview, err := svc.ParsePreview(context.Background(), 1, strings.NewReader(sampleCSV))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if preview.Rows[2].Category != "other" {
		t.Errorf("expected Splitwise's 'General' to map to 'other', got %q", preview.Rows[2].Category)
	}
	if preview.Rows[3].Category != "fuel" {
		t.Errorf("expected 'Gasolina' to map to 'fuel', got %q", preview.Rows[3].Category)
	}
}

func TestParsePreview_SkipsRowsInADifferentCurrency(t *testing.T) {
	csv := "Fecha,Descripción,Categoría,Coste,Moneda,A,B\n" +
		"2024-01-01,Cena,General,1000.00,USD,500.00,-500.00\n"
	svc, _, _, _ := newImportService(&domain.Group{ID: 1, Currency: "ARS"}, nil, nil)

	preview, err := svc.ParsePreview(context.Background(), 1, strings.NewReader(csv))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(preview.Rows) != 0 || preview.SkippedRows != 1 {
		t.Errorf("expected the USD row to be skipped, got %d rows / %d skipped", len(preview.Rows), preview.SkippedRows)
	}
	if preview.CurrencyMismatch != "USD" {
		t.Errorf("expected CurrencyMismatch %q, got %q", "USD", preview.CurrencyMismatch)
	}
}

func TestParsePreview_CurrencyMismatchNotSetWhenSomeRowsMatch(t *testing.T) {
	csv := "Fecha,Descripción,Categoría,Coste,Moneda,A,B\n" +
		"2024-01-01,Cena,General,1000.00,ARS,500.00,-500.00\n" +
		"2024-01-02,Almuerzo,General,1000.00,USD,500.00,-500.00\n"
	svc, _, _, _ := newImportService(&domain.Group{ID: 1, Currency: "ARS"}, nil, nil)

	preview, err := svc.ParsePreview(context.Background(), 1, strings.NewReader(csv))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(preview.Rows) != 1 || preview.SkippedRows != 1 {
		t.Errorf("expected 1 row parsed and 1 skipped, got %d / %d", len(preview.Rows), preview.SkippedRows)
	}
	if preview.CurrencyMismatch != "" {
		t.Errorf("expected no CurrencyMismatch when at least one row matched, got %q", preview.CurrencyMismatch)
	}
}

func TestParsePreview_RejectsGarbageInput(t *testing.T) {
	svc, _, _, _ := newImportService(&domain.Group{ID: 1, Currency: "ARS"}, nil, nil)

	if _, err := svc.ParsePreview(context.Background(), 1, strings.NewReader("not,a,csv,file")); !errors.Is(err, ErrInvalidCSV) {
		t.Fatalf("expected ErrInvalidCSV, got %v", err)
	}
}

func TestParsePreview_GroupNotFound(t *testing.T) {
	svc, _, _, _ := newImportService(nil, nil, nil)

	if _, err := svc.ParsePreview(context.Background(), 1, strings.NewReader(sampleCSV)); !errors.Is(err, ErrGroupNotFound) {
		t.Fatalf("expected ErrGroupNotFound, got %v", err)
	}
}

func TestImport_ResolvesPayerAndSplitsFromNets(t *testing.T) {
	group := &domain.Group{ID: 1, Members: []domain.User{{ID: 10}, {ID: 20}}}
	svc, _, expenseRepo, _ := newImportService(group, nil, nil)

	rows := []domain.ImportRow{
		{
			Date: time.Date(2023, 9, 24, 0, 0, 0, 0, time.UTC), Description: "Helado", Category: "groceries", AmountCents: 4000,
			MemberNets: map[string]int64{"Cami": 2000, "Hernan": -2000},
		},
	}
	mapping := map[string]uint{"Cami": 10, "Hernan": 20}

	result, err := svc.Import(context.Background(), 1, 10, rows, mapping)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Imported != 1 || result.Failed != 0 {
		t.Fatalf("expected 1 imported / 0 failed, got %+v", result)
	}
	if len(expenseRepo.created) != 1 {
		t.Fatalf("expected 1 expense created, got %d", len(expenseRepo.created))
	}

	created := expenseRepo.created[0]
	if created.expense.PaidByID != 10 {
		t.Errorf("expected Cami (10, the positive net) to be the payer, got %d", created.expense.PaidByID)
	}
	if !created.expense.CreatedAt.Equal(rows[0].Date) {
		t.Errorf("expected the original date preserved, got %v", created.expense.CreatedAt)
	}
	// A 50/50 split: the payer also owes their own half, so both members
	// appear in splits (unlike the "payer fronted the whole thing" case
	// covered by TestImport_PayerWithZeroShareIsOmittedFromSplits below).
	splitsByUser := map[uint]int64{}
	for _, sp := range created.splits {
		splitsByUser[sp.UserID] = sp.Amount
	}
	if len(created.splits) != 2 || splitsByUser[10] != 2000 || splitsByUser[20] != 2000 {
		t.Errorf("expected both members owing 2000 cents each, got %+v", created.splits)
	}
}

func TestImport_PayerWithZeroShareIsOmittedFromSplits(t *testing.T) {
	// Mirrors a real row from the reference export: one person fronts the
	// whole amount for something that's entirely the other person's share.
	group := &domain.Group{ID: 1, Members: []domain.User{{ID: 10}, {ID: 20}}}
	svc, _, expenseRepo, _ := newImportService(group, nil, nil)

	rows := []domain.ImportRow{
		{
			Date: time.Now(), Description: "Aceites", Category: "other", AmountCents: 22000,
			MemberNets: map[string]int64{"Cami": 22000, "Hernan": -22000},
		},
	}
	mapping := map[string]uint{"Cami": 10, "Hernan": 20}

	if _, err := svc.Import(context.Background(), 1, 10, rows, mapping); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	created := expenseRepo.created[0]
	if created.expense.PaidByID != 10 {
		t.Errorf("expected payer 10, got %d", created.expense.PaidByID)
	}
	if len(created.splits) != 1 || created.splits[0].UserID != 20 || created.splits[0].Amount != 22000 {
		t.Errorf("expected the full amount owed by user 20 alone, got %+v", created.splits)
	}
}

func TestImport_SkipsRowWithUnmappedPayerColumn(t *testing.T) {
	group := &domain.Group{ID: 1, Members: []domain.User{{ID: 10}, {ID: 20}}}
	svc, _, expenseRepo, _ := newImportService(group, nil, nil)

	rows := []domain.ImportRow{
		{Description: "Mystery", AmountCents: 1000, MemberNets: map[string]int64{"Unmapped": 500, "Hernan": -500}},
	}
	mapping := map[string]uint{"Hernan": 20}

	result, err := svc.Import(context.Background(), 1, 20, rows, mapping)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Imported != 0 || result.Failed != 1 {
		t.Fatalf("expected 0 imported / 1 failed, got %+v", result)
	}
	if len(expenseRepo.created) != 0 {
		t.Errorf("expected nothing created, got %d", len(expenseRepo.created))
	}
}

func TestImport_RejectsMappingToNonMember(t *testing.T) {
	group := &domain.Group{ID: 1, Members: []domain.User{{ID: 10}}}
	svc, _, _, _ := newImportService(group, nil, nil)

	rows := []domain.ImportRow{{Description: "x", AmountCents: 100, MemberNets: map[string]int64{"A": 100}}}
	mapping := map[string]uint{"A": 999} // not a member of the group

	if _, err := svc.Import(context.Background(), 1, 10, rows, mapping); err == nil {
		t.Error("expected an error when mapping to a non-member user id")
	}
}

func TestImport_RejectsNonMemberCaller(t *testing.T) {
	group := &domain.Group{ID: 1, Members: []domain.User{{ID: 10}}}
	svc, _, _, _ := newImportService(group, nil, nil)

	rows := []domain.ImportRow{{Description: "x", AmountCents: 100, MemberNets: map[string]int64{"A": 100}}}
	if _, err := svc.Import(context.Background(), 1, 99, rows, map[string]uint{"A": 10}); !errors.Is(err, ErrNotGroupMember) {
		t.Fatalf("expected ErrNotGroupMember, got %v", err)
	}
}

func TestImport_RejectsNonEmptyGroupWithExistingExpenses(t *testing.T) {
	group := &domain.Group{ID: 1, Members: []domain.User{{ID: 10}}}
	svc, _, _, _ := newImportService(group, []domain.Expense{{ID: 1, GroupID: 1}}, nil)

	rows := []domain.ImportRow{{Description: "x", AmountCents: 100, MemberNets: map[string]int64{"A": 100}}}
	if _, err := svc.Import(context.Background(), 1, 10, rows, map[string]uint{"A": 10}); !errors.Is(err, ErrGroupNotEmpty) {
		t.Fatalf("expected ErrGroupNotEmpty, got %v", err)
	}
}

func TestImport_RejectsNonEmptyGroupWithExistingSettlements(t *testing.T) {
	group := &domain.Group{ID: 1, Members: []domain.User{{ID: 10}}}
	svc, _, _, _ := newImportService(group, nil, []domain.Settlement{{ID: 1, GroupID: 1}})

	rows := []domain.ImportRow{{Description: "x", AmountCents: 100, MemberNets: map[string]int64{"A": 100}}}
	if _, err := svc.Import(context.Background(), 1, 10, rows, map[string]uint{"A": 10}); !errors.Is(err, ErrGroupNotEmpty) {
		t.Fatalf("expected ErrGroupNotEmpty, got %v", err)
	}
}

func TestImport_OneFailingRowDoesNotAbortTheRest(t *testing.T) {
	group := &domain.Group{ID: 1, Members: []domain.User{{ID: 10}, {ID: 20}}}
	groupRepo := &fakeGroupRepo{group: group}
	expenseRepo := &fakeExpenseRepo{createErr: errors.New("db exploded"), createErrOnNo: 2}
	settlementRepo := &fakeSettlementRepo{}
	svc := NewImportService(groupRepo, expenseRepo, settlementRepo)

	rows := []domain.ImportRow{
		{Description: "one", AmountCents: 1000, MemberNets: map[string]int64{"A": 500, "B": -500}},
		{Description: "two", AmountCents: 2000, MemberNets: map[string]int64{"A": 1000, "B": -1000}},
		{Description: "three", AmountCents: 3000, MemberNets: map[string]int64{"A": 1500, "B": -1500}},
	}
	mapping := map[string]uint{"A": 10, "B": 20}

	result, err := svc.Import(context.Background(), 1, 10, rows, mapping)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Imported != 2 || result.Failed != 1 {
		t.Fatalf("expected 2 imported / 1 failed, got %+v", result)
	}
}

func TestExportGroupCSV_RoundTripsExpensesAndSettlements(t *testing.T) {
	group := &domain.Group{
		ID:       1,
		Name:     "Asado!",
		Currency: "ARS",
		Members:  []domain.User{{ID: 1, Name: "Ana"}, {ID: 2, Name: "Bob"}},
	}
	expenses := []domain.Expense{{
		ID: 9, PaidByID: 1, Description: "Carne", Category: "food", Amount: 1000,
		CreatedAt: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
		Splits:    []domain.ExpenseSplit{{UserID: 1, Amount: 500}, {UserID: 2, Amount: 500}},
	}}
	settlements := []domain.Settlement{{
		ID: 3, FromUserID: 2, ToUserID: 1, Amount: 300,
		CreatedAt: time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC),
	}}
	svc, _, _, _ := newImportService(group, expenses, settlements)

	data, filename, err := svc.ExportGroupCSV(context.Background(), 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if filename != "asado-splitwise.csv" {
		t.Errorf("expected filename %q, got %q", "asado-splitwise.csv", filename)
	}

	records, err := csv.NewReader(strings.NewReader(string(data))).ReadAll()
	if err != nil {
		t.Fatalf("could not parse generated CSV: %v", err)
	}
	if len(records) != 4 {
		t.Fatalf("expected 4 records (header + total + 2 rows), got %d: %v", len(records), records)
	}

	if want := []string{"Date", "Description", "Category", "Cost", "Currency", "Ana", "Bob"}; !equalRecords(records[0], want) {
		t.Errorf("header = %v, want %v", records[0], want)
	}
	if want := []string{"", "Total balance", "", "", "", "2.00", "-2.00"}; !equalRecords(records[1], want) {
		t.Errorf("total row = %v, want %v", records[1], want)
	}
	if want := []string{"2024-01-01", "Carne", "Dining out", "10.00", "ARS", "5.00", "-5.00"}; !equalRecords(records[2], want) {
		t.Errorf("expense row = %v, want %v", records[2], want)
	}
	if want := []string{"2024-01-02", "Payment", "Payment", "3.00", "ARS", "-3.00", "3.00"}; !equalRecords(records[3], want) {
		t.Errorf("settlement row = %v, want %v", records[3], want)
	}
}

func TestExportGroupCSV_GroupNotFound(t *testing.T) {
	svc, _, _, _ := newImportService(nil, nil, nil)

	if _, _, err := svc.ExportGroupCSV(context.Background(), 1); !errors.Is(err, ErrGroupNotFound) {
		t.Errorf("expected ErrGroupNotFound, got %v", err)
	}
}

func equalRecords(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}

func TestParseCents_ParsesPositiveAndNegativeAmounts(t *testing.T) {
	cases := map[string]int64{
		"4000.00":  400000,
		"-2000.00": -200000,
		"0.00":     0,
		"15555.00": 1555500,
		"12.5":     1250,
		"12":       1200,
	}
	for input, want := range cases {
		got, err := parseCents(input)
		if err != nil {
			t.Errorf("parseCents(%q): unexpected error: %v", input, err)
			continue
		}
		if got != want {
			t.Errorf("parseCents(%q) = %d, want %d", input, got, want)
		}
	}
}

func TestParseCents_RejectsEmptyOrGarbage(t *testing.T) {
	for _, input := range []string{"", "  ", "abc", "1.2.3"} {
		if _, err := parseCents(input); err == nil {
			t.Errorf("parseCents(%q): expected an error", input)
		}
	}
}

func TestFixMojibake_RepairsDoubleEncodedUTF8(t *testing.T) {
	cases := map[string]string{
		"HernÃ¡n Iannello": "Hernán Iannello",
		"CafÃ©":            "Café",
		"Cami Vita Carino": "Cami Vita Carino", // plain ASCII — unaffected
		"Hernán Iannello":  "Hernán Iannello",  // already correct — must not be double-repaired
		"São Paulo":        "São Paulo",        // already-correct non-ASCII — must not be mangled
		"":                 "",
	}
	for input, want := range cases {
		if got := fixMojibake(input); got != want {
			t.Errorf("fixMojibake(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestMapSplitwiseCategory_KnownAndUnknown(t *testing.T) {
	cases := map[string]string{
		"Alimentos":     "groceries",
		"Restaurantes":  "food",
		"General":       "other",
		"Gasolina":      "fuel",
		"Regalos":       "gifts",
		"Groceries":     "groceries",
		"Dining out":    "food",
		"Something new": "other",
	}
	for input, want := range cases {
		if got := mapSplitwiseCategory(input); got != want {
			t.Errorf("mapSplitwiseCategory(%q) = %q, want %q", input, got, want)
		}
	}
}
