package service

import (
	"context"
	"testing"

	"spliteasy/internal/domain"
)

type fakeGroupRepoForSummary struct {
	groups []domain.Group
}

func (f *fakeGroupRepoForSummary) Create(_ context.Context, _ *domain.Group) error { return nil }

func (f *fakeGroupRepoForSummary) GetByID(_ context.Context, id uint) (*domain.Group, error) {
	for _, g := range f.groups {
		if g.ID == id {
			return &g, nil
		}
	}
	return nil, errExpected
}

func (f *fakeGroupRepoForSummary) GetByUserID(_ context.Context, _ uint) ([]domain.Group, error) {
	return f.groups, nil
}

func (f *fakeGroupRepoForSummary) GetByInviteToken(_ context.Context, _ string) (*domain.Group, error) {
	return nil, errExpected
}

func (f *fakeGroupRepoForSummary) AddMember(_ context.Context, _, _ uint) error { return nil }

func (f *fakeGroupRepoForSummary) SetInviteTokenIfEmpty(_ context.Context, _ uint, _ string) error {
	return nil
}

// fakeExpenseRepoByGroup and fakeSettlementRepoByGroup, unlike the
// same-named fakes in balance_service_test.go, actually filter by group id
// — GetHomeSummary iterates multiple groups per test and needs each one to
// see only its own expenses/settlements.
type fakeExpenseRepoByGroup struct {
	byGroup map[uint][]domain.Expense
}

func (f *fakeExpenseRepoByGroup) CreateWithSplits(_ context.Context, _ *domain.Expense, _ []domain.ExpenseSplit, _ []domain.ExpenseItem) error {
	return nil
}
func (f *fakeExpenseRepoByGroup) UpdateWithSplits(_ context.Context, _ *domain.Expense, _ []domain.ExpenseSplit, _ []domain.ExpenseItem) error {
	return nil
}
func (f *fakeExpenseRepoByGroup) GetByID(_ context.Context, _ uint) (*domain.Expense, error) {
	return nil, errExpected
}
func (f *fakeExpenseRepoByGroup) GetByGroupID(_ context.Context, groupID uint) ([]domain.Expense, error) {
	return f.byGroup[groupID], nil
}
func (f *fakeExpenseRepoByGroup) Delete(_ context.Context, _ uint) error { return nil }

type fakeSettlementRepoByGroup struct {
	byGroup map[uint][]domain.Settlement
}

func (f *fakeSettlementRepoByGroup) Create(_ context.Context, _ *domain.Settlement) error {
	return nil
}
func (f *fakeSettlementRepoByGroup) GetByID(_ context.Context, _ uint) (*domain.Settlement, error) {
	return nil, errExpected
}
func (f *fakeSettlementRepoByGroup) GetByGroupID(_ context.Context, groupID uint) ([]domain.Settlement, error) {
	return f.byGroup[groupID], nil
}
func (f *fakeSettlementRepoByGroup) Delete(_ context.Context, _ uint) error { return nil }

func TestGetHomeSummary_BreaksDownByCurrency(t *testing.T) {
	groups := []domain.Group{
		{ID: 1, Name: "USD Trip", Currency: "USD", Members: []domain.User{{ID: 1}, {ID: 2}}},
		{ID: 2, Name: "ARS Asado", Currency: "ARS", Members: []domain.User{{ID: 1}, {ID: 2}}},
	}
	expensesByGroup := map[uint][]domain.Expense{
		// Group 1 (USD): user 1 paid 100, split evenly — user 1 is owed 50.
		1: {{PaidByID: 1, Amount: 100, Splits: []domain.ExpenseSplit{{UserID: 1, Amount: 50}, {UserID: 2, Amount: 50}}}},
		// Group 2 (ARS): user 2 paid 4000, split evenly — user 1 owes 2000.
		2: {{PaidByID: 2, Amount: 4000, Splits: []domain.ExpenseSplit{{UserID: 1, Amount: 2000}, {UserID: 2, Amount: 2000}}}},
	}

	svc := NewSummaryService(
		&fakeGroupRepoForSummary{groups: groups},
		&fakeExpenseRepoByGroup{byGroup: expensesByGroup},
		&fakeSettlementRepoByGroup{},
	)

	summary, err := svc.GetHomeSummary(context.Background(), 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(summary.OverallByCurrency) != 2 {
		t.Fatalf("expected 2 currency totals, got %d: %+v", len(summary.OverallByCurrency), summary.OverallByCurrency)
	}

	byCurrency := map[string]domain.OverallBalance{}
	for _, o := range summary.OverallByCurrency {
		byCurrency[o.Currency] = o
	}

	usd := byCurrency["USD"]
	if usd.Net != 50 || usd.Owed != 50 || usd.Owe != 0 {
		t.Errorf("unexpected USD total: %+v", usd)
	}
	ars := byCurrency["ARS"]
	if ars.Net != -2000 || ars.Owed != 0 || ars.Owe != 2000 {
		t.Errorf("unexpected ARS total: %+v", ars)
	}

	if len(summary.Groups) != 2 {
		t.Fatalf("expected 2 group summaries, got %d", len(summary.Groups))
	}
	for _, g := range summary.Groups {
		if g.Currency == "" {
			t.Errorf("expected group summary to carry its currency, got empty for group %d", g.ID)
		}
	}
}

func TestGetActivity_CarriesGroupCurrency(t *testing.T) {
	groups := []domain.Group{
		{ID: 1, Name: "ARS Asado", Currency: "ARS", Members: []domain.User{{ID: 1}}},
	}
	expensesByGroup := map[uint][]domain.Expense{
		1: {{ID: 9, PaidByID: 1, Description: "Carne", Amount: 5000}},
	}

	svc := NewSummaryService(
		&fakeGroupRepoForSummary{groups: groups},
		&fakeExpenseRepoByGroup{byGroup: expensesByGroup},
		&fakeSettlementRepoByGroup{},
	)

	events, err := svc.GetActivity(context.Background(), 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].Currency != "ARS" {
		t.Errorf("expected event currency %q, got %q", "ARS", events[0].Currency)
	}
}
