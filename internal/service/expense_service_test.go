package service

import (
	"context"
	"testing"

	"spliteasy/internal/domain"
)

type fakeExpenseRepoForCreate struct {
	createdExpense *domain.Expense
	createdSplits  []domain.ExpenseSplit
}

func (f *fakeExpenseRepoForCreate) CreateWithSplits(_ context.Context, expense *domain.Expense, splits []domain.ExpenseSplit) error {
	expense.ID = 1
	f.createdExpense = expense
	f.createdSplits = splits
	return nil
}

func (f *fakeExpenseRepoForCreate) GetByGroupID(_ context.Context, groupID uint) ([]domain.Expense, error) {
	return nil, nil
}

func newTestExpenseService(members []domain.User) (ExpenseService, *fakeExpenseRepoForCreate) {
	expenseRepo := &fakeExpenseRepoForCreate{}
	svc := NewExpenseService(expenseRepo, &fakeGroupRepo{group: &domain.Group{ID: 1, Members: members}})
	return svc, expenseRepo
}

func splitsByUser(splits []domain.ExpenseSplit) map[uint]float64 {
	result := make(map[uint]float64, len(splits))
	for _, s := range splits {
		result[s.UserID] = s.Amount
	}
	return result
}

func TestAddExpense_EqualAmongAllMembers(t *testing.T) {
	members := []domain.User{{ID: 1}, {ID: 2}}
	svc, repo := newTestExpenseService(members)

	_, err := svc.AddExpense(context.Background(), 1, 1, "Dinner", 100, SplitEqual, nil)
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

	_, err := svc.AddExpense(context.Background(), 1, 1, "Dinner", 100, SplitEqual, []SplitInput{{UserID: 1}, {UserID: 2}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	splits := splitsByUser(repo.createdSplits)
	if len(splits) != 2 || splits[1] != 50 || splits[2] != 50 {
		t.Errorf("expected 50/50 split among 2 members, got %+v", splits)
	}
}

func TestAddExpense_Percentage(t *testing.T) {
	members := []domain.User{{ID: 1}, {ID: 2}}
	svc, repo := newTestExpenseService(members)

	_, err := svc.AddExpense(context.Background(), 1, 1, "Rent", 1000, SplitPercentage, []SplitInput{
		{UserID: 1, Value: 70},
		{UserID: 2, Value: 30},
	})
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

	_, err := svc.AddExpense(context.Background(), 1, 1, "Rent", 1000, SplitPercentage, []SplitInput{
		{UserID: 1, Value: 70},
		{UserID: 2, Value: 20},
	})
	if err == nil {
		t.Error("expected error when percentages don't add up to 100")
	}
}

func TestAddExpense_Fixed(t *testing.T) {
	members := []domain.User{{ID: 1}, {ID: 2}}
	svc, repo := newTestExpenseService(members)

	_, err := svc.AddExpense(context.Background(), 1, 1, "Groceries", 300, SplitFixed, []SplitInput{
		{UserID: 1, Value: 100},
		{UserID: 2, Value: 200},
	})
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

	_, err := svc.AddExpense(context.Background(), 1, 1, "Groceries", 300, SplitFixed, []SplitInput{
		{UserID: 1, Value: 100},
		{UserID: 2, Value: 150},
	})
	if err == nil {
		t.Error("expected error when fixed amounts don't add up to the total")
	}
}

func TestAddExpense_Shares(t *testing.T) {
	members := []domain.User{{ID: 1}, {ID: 2}}
	svc, repo := newTestExpenseService(members)

	_, err := svc.AddExpense(context.Background(), 1, 1, "Bread", 30, SplitShares, []SplitInput{
		{UserID: 1, Value: 2},
		{UserID: 2, Value: 4},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	splits := splitsByUser(repo.createdSplits)
	if splits[1] != 10 || splits[2] != 20 {
		t.Errorf("expected 10/20 split (1:2 ratio), got %+v", splits)
	}
}

func TestAddExpense_RejectsNonMemberInSplit(t *testing.T) {
	members := []domain.User{{ID: 1}, {ID: 2}}
	svc, _ := newTestExpenseService(members)

	_, err := svc.AddExpense(context.Background(), 1, 1, "Dinner", 100, SplitEqual, []SplitInput{{UserID: 1}, {UserID: 99}})
	if err == nil {
		t.Error("expected error when split includes a non-member")
	}
}

func TestAddExpense_RejectsNonMemberPayer(t *testing.T) {
	members := []domain.User{{ID: 1}, {ID: 2}}
	svc, _ := newTestExpenseService(members)

	_, err := svc.AddExpense(context.Background(), 1, 99, "Dinner", 100, SplitEqual, nil)
	if err == nil {
		t.Error("expected error when payer is not a member")
	}
}

func TestAddExpense_RejectsNonPositiveAmount(t *testing.T) {
	members := []domain.User{{ID: 1}, {ID: 2}}
	svc, _ := newTestExpenseService(members)

	_, err := svc.AddExpense(context.Background(), 1, 1, "Dinner", 0, SplitEqual, nil)
	if err == nil {
		t.Error("expected error for non-positive amount")
	}
}
