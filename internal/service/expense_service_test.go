package service

import (
	"context"
	"errors"
	"testing"

	"spliteasy/internal/domain"
)

type fakeExpenseRepoForCreate struct {
	createdExpense *domain.Expense
	createdSplits  []domain.ExpenseSplit
	createdItems   []domain.ExpenseItem

	// existing, when set, is what GetByID returns — used to test
	// UpdateExpense/DeleteExpense against a pre-existing expense.
	existing *domain.Expense

	updatedExpense *domain.Expense
	updatedSplits  []domain.ExpenseSplit
	updatedItems   []domain.ExpenseItem

	deletedID uint
}

func (f *fakeExpenseRepoForCreate) CreateWithSplits(_ context.Context, expense *domain.Expense, splits []domain.ExpenseSplit, items []domain.ExpenseItem) error {
	expense.ID = 1
	f.createdExpense = expense
	f.createdSplits = splits
	f.createdItems = items
	return nil
}

func (f *fakeExpenseRepoForCreate) UpdateWithSplits(_ context.Context, expense *domain.Expense, splits []domain.ExpenseSplit, items []domain.ExpenseItem) error {
	f.updatedExpense = expense
	f.updatedSplits = splits
	f.updatedItems = items
	return nil
}

func (f *fakeExpenseRepoForCreate) GetByID(_ context.Context, id uint) (*domain.Expense, error) {
	if f.existing == nil || f.existing.ID != id {
		return nil, errExpected
	}
	return f.existing, nil
}

func (f *fakeExpenseRepoForCreate) GetByGroupID(_ context.Context, groupID uint) ([]domain.Expense, error) {
	return nil, nil
}

func (f *fakeExpenseRepoForCreate) Delete(_ context.Context, id uint) error {
	f.deletedID = id
	return nil
}

func newTestExpenseService(members []domain.User) (ExpenseService, *fakeExpenseRepoForCreate) {
	expenseRepo := &fakeExpenseRepoForCreate{}
	svc := NewExpenseService(expenseRepo, &fakeGroupRepo{group: &domain.Group{ID: 1, Members: members}})
	return svc, expenseRepo
}

func splitsByUser(splits []domain.ExpenseSplit) map[uint]int64 {
	result := make(map[uint]int64, len(splits))
	for _, s := range splits {
		result[s.UserID] = s.Amount
	}
	return result
}

func TestAddExpense_EqualAmongAllMembers(t *testing.T) {
	members := []domain.User{{ID: 1}, {ID: 2}}
	svc, repo := newTestExpenseService(members)

	_, err := svc.AddExpense(context.Background(), 1, 1, "Dinner", "", 100, SplitEqual, nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	splits := splitsByUser(repo.createdSplits)
	if splits[1] != 50 || splits[2] != 50 {
		t.Errorf("expected 50/50 split, got %+v", splits)
	}
}

func TestAddExpense_EqualAmongSubset(t *testing.T) {
	members := []domain.User{{ID: 1}, {ID: 2}, {ID: 3}}
	svc, repo := newTestExpenseService(members)

	_, err := svc.AddExpense(context.Background(), 1, 1, "Dinner", "", 100, SplitEqual, []SplitInput{{UserID: 1}, {UserID: 2}}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	splits := splitsByUser(repo.createdSplits)
	if len(splits) != 2 || splits[1] != 50 || splits[2] != 50 {
		t.Errorf("expected 50/50 split among 2 members, got %+v", splits)
	}
}

func TestAddExpense_EqualDistributesRemainderExactly(t *testing.T) {
	members := []domain.User{{ID: 1}, {ID: 2}, {ID: 3}}
	svc, repo := newTestExpenseService(members)

	// 1000 cents / 3 doesn't divide evenly; the parts must still sum to 1000.
	_, err := svc.AddExpense(context.Background(), 1, 1, "Dinner", "", 1000, SplitEqual, nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	splits := splitsByUser(repo.createdSplits)
	var total int64
	for _, c := range splits {
		total += c
	}
	if total != 1000 {
		t.Errorf("expected splits to sum to 1000 cents, got %d", total)
	}
	// The extra cent goes to the lowest user id (deterministic tie-break).
	if splits[1] != 334 || splits[2] != 333 || splits[3] != 333 {
		t.Errorf("expected 334/333/333, got %+v", splits)
	}
}

func TestAddExpense_Percentage(t *testing.T) {
	members := []domain.User{{ID: 1}, {ID: 2}}
	svc, repo := newTestExpenseService(members)

	_, err := svc.AddExpense(context.Background(), 1, 1, "Rent", "", 1000, SplitPercentage, []SplitInput{
		{UserID: 1, Value: 70},
		{UserID: 2, Value: 30},
	}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	splits := splitsByUser(repo.createdSplits)
	if splits[1] != 700 || splits[2] != 300 {
		t.Errorf("expected 700/300 split, got %+v", splits)
	}
}

func TestAddExpense_PercentageMustAddUpTo100(t *testing.T) {
	members := []domain.User{{ID: 1}, {ID: 2}}
	svc, _ := newTestExpenseService(members)

	_, err := svc.AddExpense(context.Background(), 1, 1, "Rent", "", 1000, SplitPercentage, []SplitInput{
		{UserID: 1, Value: 70},
		{UserID: 2, Value: 20},
	}, nil)
	if err == nil {
		t.Error("expected error when percentages don't add up to 100")
	}
}

func TestAddExpense_PercentageRejectsNegativeValue(t *testing.T) {
	members := []domain.User{{ID: 1}, {ID: 2}}
	svc, _ := newTestExpenseService(members)

	// Sums to 100 (-900 + 1000), which would otherwise pass the "adds up to
	// 100" check while giving user 1 a negative split — i.e. a credit — and
	// user 2 a share ten times the actual expense amount.
	_, err := svc.AddExpense(context.Background(), 1, 1, "Rent", "", 100, SplitPercentage, []SplitInput{
		{UserID: 1, Value: -900},
		{UserID: 2, Value: 1000},
	}, nil)
	if err == nil {
		t.Error("expected error for a negative percentage")
	}
}

func TestAddExpense_Fixed(t *testing.T) {
	members := []domain.User{{ID: 1}, {ID: 2}}
	svc, repo := newTestExpenseService(members)

	_, err := svc.AddExpense(context.Background(), 1, 1, "Groceries", "", 300, SplitFixed, []SplitInput{
		{UserID: 1, Value: 100},
		{UserID: 2, Value: 200},
	}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	splits := splitsByUser(repo.createdSplits)
	if splits[1] != 100 || splits[2] != 200 {
		t.Errorf("expected 100/200 split, got %+v", splits)
	}
}

func TestAddExpense_FixedMustAddUpToAmount(t *testing.T) {
	members := []domain.User{{ID: 1}, {ID: 2}}
	svc, _ := newTestExpenseService(members)

	_, err := svc.AddExpense(context.Background(), 1, 1, "Groceries", "", 300, SplitFixed, []SplitInput{
		{UserID: 1, Value: 100},
		{UserID: 2, Value: 150},
	}, nil)
	if err == nil {
		t.Error("expected error when fixed amounts don't add up to the total")
	}
}

func TestAddExpense_FixedRejectsNegativeValue(t *testing.T) {
	members := []domain.User{{ID: 1}, {ID: 2}}
	svc, _ := newTestExpenseService(members)

	// Sums to the expense total (300 = -200 + 500), which would otherwise
	// pass the "adds up to the total" check while crediting user 1 instead
	// of charging them.
	_, err := svc.AddExpense(context.Background(), 1, 1, "Groceries", "", 300, SplitFixed, []SplitInput{
		{UserID: 1, Value: -200},
		{UserID: 2, Value: 500},
	}, nil)
	if err == nil {
		t.Error("expected error for a negative fixed amount")
	}
}

func TestAddExpense_Shares(t *testing.T) {
	members := []domain.User{{ID: 1}, {ID: 2}}
	svc, repo := newTestExpenseService(members)

	_, err := svc.AddExpense(context.Background(), 1, 1, "Bread", "", 30, SplitShares, []SplitInput{
		{UserID: 1, Value: 2},
		{UserID: 2, Value: 4},
	}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	splits := splitsByUser(repo.createdSplits)
	if splits[1] != 10 || splits[2] != 20 {
		t.Errorf("expected 10/20 split (1:2 ratio), got %+v", splits)
	}
}

func TestAddExpense_PersistsItems(t *testing.T) {
	members := []domain.User{{ID: 1}, {ID: 2}, {ID: 3}}
	svc, repo := newTestExpenseService(members)

	// Fixed split so the per-person amounts equal the item assignments.
	_, err := svc.AddExpense(context.Background(), 1, 1, "Dinner", "", 300, SplitFixed,
		[]SplitInput{{UserID: 1, Value: 100}, {UserID: 2, Value: 100}, {UserID: 3, Value: 100}},
		[]ItemInput{
			{Description: "Burger", Amount: 200, UserIDs: []uint{1, 2}},
			{Description: "Salad", Amount: 100, UserIDs: []uint{3}},
		},
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(repo.createdItems) != 2 {
		t.Fatalf("expected 2 items persisted, got %d", len(repo.createdItems))
	}
	if repo.createdItems[0].Description != "Burger" || len(repo.createdItems[0].Users) != 2 {
		t.Errorf("unexpected first item: %+v", repo.createdItems[0])
	}
}

func TestAddExpense_RejectsItemForNonMember(t *testing.T) {
	members := []domain.User{{ID: 1}, {ID: 2}}
	svc, _ := newTestExpenseService(members)

	_, err := svc.AddExpense(context.Background(), 1, 1, "Dinner", "", 100, SplitEqual, nil,
		[]ItemInput{{Description: "Burger", Amount: 100, UserIDs: []uint{99}}},
	)
	if err == nil {
		t.Error("expected error when an item is assigned to a non-member")
	}
}

func TestAddExpense_RejectsNonMemberInSplit(t *testing.T) {
	members := []domain.User{{ID: 1}, {ID: 2}}
	svc, _ := newTestExpenseService(members)

	_, err := svc.AddExpense(context.Background(), 1, 1, "Dinner", "", 100, SplitEqual, []SplitInput{{UserID: 1}, {UserID: 99}}, nil)
	if err == nil {
		t.Error("expected error when split includes a non-member")
	}
}

func TestAddExpense_RejectsNonMemberPayer(t *testing.T) {
	members := []domain.User{{ID: 1}, {ID: 2}}
	svc, _ := newTestExpenseService(members)

	_, err := svc.AddExpense(context.Background(), 1, 99, "Dinner", "", 100, SplitEqual, nil, nil)
	if err == nil {
		t.Error("expected error when payer is not a member")
	}
}

func TestAddExpense_RejectsNonPositiveAmount(t *testing.T) {
	members := []domain.User{{ID: 1}, {ID: 2}}
	svc, _ := newTestExpenseService(members)

	_, err := svc.AddExpense(context.Background(), 1, 1, "Dinner", "", 0, SplitEqual, nil, nil)
	if err == nil {
		t.Error("expected error for non-positive amount")
	}
}

func TestAddExpense_DefaultsCategoryToOther(t *testing.T) {
	svc, repo := newTestExpenseService([]domain.User{{ID: 1}})

	if _, err := svc.AddExpense(context.Background(), 1, 1, "Dinner", "", 100, SplitEqual, nil, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if repo.createdExpense.Category != domain.DefaultExpenseCategory {
		t.Errorf("expected default category %q, got %q", domain.DefaultExpenseCategory, repo.createdExpense.Category)
	}
}

func TestAddExpense_PersistsCategory(t *testing.T) {
	svc, repo := newTestExpenseService([]domain.User{{ID: 1}})

	if _, err := svc.AddExpense(context.Background(), 1, 1, "Dinner", "food", 100, SplitEqual, nil, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if repo.createdExpense.Category != "food" {
		t.Errorf("expected category %q, got %q", "food", repo.createdExpense.Category)
	}
}

func TestAddExpense_RejectsUnknownCategory(t *testing.T) {
	svc, repo := newTestExpenseService([]domain.User{{ID: 1}})

	if _, err := svc.AddExpense(context.Background(), 1, 1, "Dinner", "yachts", 100, SplitEqual, nil, nil); err == nil {
		t.Error("expected error for unknown category")
	}
	if repo.createdExpense != nil {
		t.Error("expected no expense to be persisted")
	}
}

func existingTwoPersonExpense() *domain.Expense {
	return &domain.Expense{
		ID: 5, GroupID: 1, PaidByID: 1, Amount: 1000,
		Splits: []domain.ExpenseSplit{{UserID: 1, Amount: 500}, {UserID: 2, Amount: 500}},
	}
}

func TestUpdateExpense_AllowsPayerToEdit(t *testing.T) {
	members := []domain.User{{ID: 1}, {ID: 2}}
	svc, repo := newTestExpenseService(members)
	repo.existing = existingTwoPersonExpense()

	updated, err := svc.UpdateExpense(context.Background(), 5, 1, 1, "Dinner (edited)", "", 2000, SplitEqual, nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if updated.Amount != 2000 || updated.Description != "Dinner (edited)" {
		t.Errorf("unexpected updated expense: %+v", updated)
	}
	splits := splitsByUser(repo.updatedSplits)
	if splits[1] != 1000 || splits[2] != 1000 {
		t.Errorf("expected an equal split of the new amount, got %+v", splits)
	}
}

func TestUpdateExpense_ChangesCategory(t *testing.T) {
	members := []domain.User{{ID: 1}, {ID: 2}}
	svc, repo := newTestExpenseService(members)
	repo.existing = existingTwoPersonExpense()

	updated, err := svc.UpdateExpense(context.Background(), 5, 1, 1, "Dinner", "food", 1000, SplitEqual, nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if updated.Category != "food" || repo.updatedExpense.Category != "food" {
		t.Errorf("expected category to change to %q, got %q (persisted %q)", "food", updated.Category, repo.updatedExpense.Category)
	}
}

func TestUpdateExpense_RejectsUnknownCategory(t *testing.T) {
	members := []domain.User{{ID: 1}, {ID: 2}}
	svc, repo := newTestExpenseService(members)
	repo.existing = existingTwoPersonExpense()

	if _, err := svc.UpdateExpense(context.Background(), 5, 1, 1, "Dinner", "yachts", 1000, SplitEqual, nil, nil); err == nil {
		t.Error("expected error for unknown category")
	}
	if repo.updatedExpense != nil {
		t.Error("expected no update to be persisted")
	}
}

func TestUpdateExpense_AllowsSplitParticipantToEdit(t *testing.T) {
	members := []domain.User{{ID: 1}, {ID: 2}}
	svc, repo := newTestExpenseService(members)
	repo.existing = existingTwoPersonExpense()

	// Caller (2) didn't pay, but is one of the split participants.
	if _, err := svc.UpdateExpense(context.Background(), 5, 2, 1, "Dinner", "", 1000, SplitEqual, nil, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestUpdateExpense_RejectsBystander(t *testing.T) {
	members := []domain.User{{ID: 1}, {ID: 2}, {ID: 3}}
	svc, repo := newTestExpenseService(members)
	repo.existing = existingTwoPersonExpense()

	_, err := svc.UpdateExpense(context.Background(), 5, 3, 1, "Dinner", "", 1000, SplitEqual, nil, nil)
	if !errors.Is(err, ErrNotExpenseParty) {
		t.Errorf("expected ErrNotExpenseParty, got %v", err)
	}
}

func TestUpdateExpense_RejectsUnknownExpense(t *testing.T) {
	members := []domain.User{{ID: 1}, {ID: 2}}
	svc, _ := newTestExpenseService(members)

	_, err := svc.UpdateExpense(context.Background(), 999, 1, 1, "Dinner", "", 1000, SplitEqual, nil, nil)
	if !errors.Is(err, ErrExpenseNotFound) {
		t.Errorf("expected ErrExpenseNotFound, got %v", err)
	}
}

func TestUpdateExpense_StillValidatesSplits(t *testing.T) {
	members := []domain.User{{ID: 1}, {ID: 2}}
	svc, repo := newTestExpenseService(members)
	repo.existing = existingTwoPersonExpense()

	_, err := svc.UpdateExpense(context.Background(), 5, 1, 1, "Dinner", "", 100, SplitPercentage, []SplitInput{
		{UserID: 1, Value: -900},
		{UserID: 2, Value: 1000},
	}, nil)
	if err == nil {
		t.Error("expected the same negative-percentage validation as AddExpense on update")
	}
}

func TestDeleteExpense_AllowsPayerToDelete(t *testing.T) {
	members := []domain.User{{ID: 1}, {ID: 2}}
	svc, repo := newTestExpenseService(members)
	repo.existing = existingTwoPersonExpense()

	if err := svc.DeleteExpense(context.Background(), 5, 1); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if repo.deletedID != 5 {
		t.Errorf("expected Delete to be called with id 5, got %d", repo.deletedID)
	}
}

func TestDeleteExpense_AllowsSplitParticipantToDelete(t *testing.T) {
	members := []domain.User{{ID: 1}, {ID: 2}}
	svc, repo := newTestExpenseService(members)
	repo.existing = existingTwoPersonExpense()

	if err := svc.DeleteExpense(context.Background(), 5, 2); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDeleteExpense_RejectsBystander(t *testing.T) {
	members := []domain.User{{ID: 1}, {ID: 2}, {ID: 3}}
	svc, repo := newTestExpenseService(members)
	repo.existing = existingTwoPersonExpense()

	err := svc.DeleteExpense(context.Background(), 5, 3)
	if !errors.Is(err, ErrNotExpenseParty) {
		t.Errorf("expected ErrNotExpenseParty, got %v", err)
	}
	if repo.deletedID != 0 {
		t.Error("expected Delete not to be called")
	}
}

func TestDeleteExpense_RejectsUnknownExpense(t *testing.T) {
	members := []domain.User{{ID: 1}, {ID: 2}}
	svc, _ := newTestExpenseService(members)

	err := svc.DeleteExpense(context.Background(), 999, 1)
	if !errors.Is(err, ErrExpenseNotFound) {
		t.Errorf("expected ErrExpenseNotFound, got %v", err)
	}
}

func TestGetExpense_ReturnsAnyGroupMemberTheExpense(t *testing.T) {
	// No payer/participant restriction here — GetExpense is a plain read,
	// unlike Update/Delete.
	members := []domain.User{{ID: 1}, {ID: 2}, {ID: 3}}
	svc, repo := newTestExpenseService(members)
	repo.existing = existingTwoPersonExpense()

	got, err := svc.GetExpense(context.Background(), 5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.ID != 5 {
		t.Errorf("expected expense id 5, got %d", got.ID)
	}
}

func TestGetExpense_RejectsUnknownExpense(t *testing.T) {
	members := []domain.User{{ID: 1}, {ID: 2}}
	svc, _ := newTestExpenseService(members)

	_, err := svc.GetExpense(context.Background(), 999)
	if !errors.Is(err, ErrExpenseNotFound) {
		t.Errorf("expected ErrExpenseNotFound, got %v", err)
	}
}
