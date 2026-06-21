package service

import (
	"errors"
	"testing"

	"spliteasy/internal/domain"
)

type fakeExpenseRepo struct {
	expenses []domain.Expense
}

func (f *fakeExpenseRepo) CreateWithSplits(expense *domain.Expense, splits []domain.ExpenseSplit) error {
	return nil
}

func (f *fakeExpenseRepo) GetByGroupID(groupID uint) ([]domain.Expense, error) {
	return f.expenses, nil
}

type fakeGroupRepo struct {
	group *domain.Group
}

func (f *fakeGroupRepo) Create(group *domain.Group) error { return nil }

func (f *fakeGroupRepo) GetByID(id uint) (*domain.Group, error) {
	if f.group == nil {
		return nil, errors.New("not found")
	}
	return f.group, nil
}

func (f *fakeGroupRepo) GetByUserID(userID uint) ([]domain.Group, error) { return nil, nil }

type fakeSettlementRepo struct {
	settlements []domain.Settlement
	created     []*domain.Settlement
}

func (f *fakeSettlementRepo) Create(settlement *domain.Settlement) error {
	settlement.ID = uint(len(f.created) + 1)
	f.created = append(f.created, settlement)
	return nil
}

func (f *fakeSettlementRepo) GetByGroupID(groupID uint) ([]domain.Settlement, error) {
	return f.settlements, nil
}

func newTestBalanceService(expenses []domain.Expense, settlements []domain.Settlement) (BalanceService, *fakeSettlementRepo) {
	settlementRepo := &fakeSettlementRepo{settlements: settlements}
	svc := NewBalanceService(
		&fakeExpenseRepo{expenses: expenses},
		&fakeGroupRepo{group: &domain.Group{ID: 1}},
		settlementRepo,
	)
	return svc, settlementRepo
}

func TestCalculateGroupDebts_NoSettlements(t *testing.T) {
	expenses := []domain.Expense{
		{
			PaidByID: 1,
			Amount:   100,
			Splits: []domain.ExpenseSplit{
				{UserID: 1, Amount: 50},
				{UserID: 2, Amount: 50},
			},
		},
	}

	svc, _ := newTestBalanceService(expenses, nil)

	debts, err := svc.CalculateGroupDebts(1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(debts) != 1 {
		t.Fatalf("expected 1 debt, got %d", len(debts))
	}
	if debts[0].FromUserID != 2 || debts[0].ToUserID != 1 || debts[0].Amount != 50 {
		t.Errorf("unexpected debt: %+v", debts[0])
	}
}

func TestCalculateGroupDebts_SettlementReducesDebt(t *testing.T) {
	expenses := []domain.Expense{
		{
			PaidByID: 1,
			Amount:   100,
			Splits: []domain.ExpenseSplit{
				{UserID: 1, Amount: 50},
				{UserID: 2, Amount: 50},
			},
		},
	}
	settlements := []domain.Settlement{
		{GroupID: 1, FromUserID: 2, ToUserID: 1, Amount: 30},
	}

	svc, _ := newTestBalanceService(expenses, settlements)

	debts, err := svc.CalculateGroupDebts(1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(debts) != 1 {
		t.Fatalf("expected 1 remaining debt, got %d", len(debts))
	}
	if debts[0].Amount != 20 {
		t.Errorf("expected remaining debt of 20, got %v", debts[0].Amount)
	}
}

func TestCalculateGroupDebts_FullySettled(t *testing.T) {
	expenses := []domain.Expense{
		{
			PaidByID: 1,
			Amount:   100,
			Splits: []domain.ExpenseSplit{
				{UserID: 1, Amount: 50},
				{UserID: 2, Amount: 50},
			},
		},
	}
	settlements := []domain.Settlement{
		{GroupID: 1, FromUserID: 2, ToUserID: 1, Amount: 50},
	}

	svc, _ := newTestBalanceService(expenses, settlements)

	debts, err := svc.CalculateGroupDebts(1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(debts) != 0 {
		t.Errorf("expected no remaining debts, got %+v", debts)
	}
}

func TestSettleDebt_RejectsNonPositiveAmount(t *testing.T) {
	svc, _ := newTestBalanceService(nil, nil)

	if _, err := svc.SettleDebt(1, 2, 1, 0); err == nil {
		t.Error("expected error for zero amount")
	}
	if _, err := svc.SettleDebt(1, 2, 1, -10); err == nil {
		t.Error("expected error for negative amount")
	}
}

func TestSettleDebt_RejectsSameUser(t *testing.T) {
	svc, _ := newTestBalanceService(nil, nil)

	if _, err := svc.SettleDebt(1, 1, 1, 10); err == nil {
		t.Error("expected error when from_user_id equals to_user_id")
	}
}

func TestSettleDebt_PersistsSettlement(t *testing.T) {
	svc, settlementRepo := newTestBalanceService(nil, nil)

	settlement, err := svc.SettleDebt(1, 2, 1, 30)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if settlement.GroupID != 1 || settlement.FromUserID != 2 || settlement.ToUserID != 1 || settlement.Amount != 30 {
		t.Errorf("unexpected settlement: %+v", settlement)
	}
	if len(settlementRepo.created) != 1 {
		t.Fatalf("expected settlement to be persisted, got %d", len(settlementRepo.created))
	}
}

func TestSettleDebt_GroupNotFound(t *testing.T) {
	settlementRepo := &fakeSettlementRepo{}
	svc := NewBalanceService(
		&fakeExpenseRepo{},
		&fakeGroupRepo{group: nil},
		settlementRepo,
	)

	if _, err := svc.SettleDebt(1, 2, 1, 30); err == nil {
		t.Error("expected error when group does not exist")
	}
}
