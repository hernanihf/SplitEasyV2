package service

import (
	"context"
	"errors"
	"testing"

	"spliteasy/internal/domain"
)

type fakeExpenseRepo struct {
	expenses []domain.Expense
}

func (f *fakeExpenseRepo) CreateWithSplits(_ context.Context, expense *domain.Expense, splits []domain.ExpenseSplit, _ []domain.ExpenseItem) error {
	return nil
}

func (f *fakeExpenseRepo) UpdateWithSplits(_ context.Context, _ *domain.Expense, _ []domain.ExpenseSplit, _ []domain.ExpenseItem) error {
	return nil
}

func (f *fakeExpenseRepo) GetByID(_ context.Context, id uint) (*domain.Expense, error) {
	for _, e := range f.expenses {
		if e.ID == id {
			return &e, nil
		}
	}
	return nil, errExpected
}

func (f *fakeExpenseRepo) GetByGroupID(_ context.Context, groupID uint) ([]domain.Expense, error) {
	return f.expenses, nil
}

func (f *fakeExpenseRepo) Delete(_ context.Context, _ uint) error {
	return nil
}

type fakeGroupRepo struct {
	group         *domain.Group
	addedMembers  [][2]uint
	updatedTokens map[uint]string
}

func (f *fakeGroupRepo) Create(_ context.Context, group *domain.Group) error { return nil }

func (f *fakeGroupRepo) GetByID(_ context.Context, id uint) (*domain.Group, error) {
	if f.group == nil {
		return nil, errors.New("not found")
	}
	return f.group, nil
}

func (f *fakeGroupRepo) GetByUserID(_ context.Context, userID uint) ([]domain.Group, error) {
	return nil, nil
}

func (f *fakeGroupRepo) GetByInviteToken(_ context.Context, token string) (*domain.Group, error) {
	if f.group == nil {
		return nil, errors.New("not found")
	}
	return f.group, nil
}

func (f *fakeGroupRepo) AddMember(_ context.Context, groupID, userID uint) error {
	f.addedMembers = append(f.addedMembers, [2]uint{groupID, userID})
	return nil
}

func (f *fakeGroupRepo) SetInviteTokenIfEmpty(_ context.Context, groupID uint, token string) error {
	if f.updatedTokens == nil {
		f.updatedTokens = map[uint]string{}
	}
	// Mimic the conditional write: only set when currently empty, and reflect it
	// on the group so a subsequent GetByID returns the persisted token.
	if f.group != nil && f.group.ID == groupID && f.group.InviteToken == "" {
		f.group.InviteToken = token
		f.updatedTokens[groupID] = token
	}
	return nil
}

type fakeSettlementRepo struct {
	settlements []domain.Settlement
	created     []*domain.Settlement
	deletedID   uint
}

func (f *fakeSettlementRepo) Create(_ context.Context, settlement *domain.Settlement) error {
	settlement.ID = uint(len(f.created) + 1)
	f.created = append(f.created, settlement)
	return nil
}

func (f *fakeSettlementRepo) GetByID(_ context.Context, id uint) (*domain.Settlement, error) {
	for _, s := range f.settlements {
		if s.ID == id {
			return &s, nil
		}
	}
	return nil, errExpected
}

func (f *fakeSettlementRepo) GetByGroupID(_ context.Context, groupID uint) ([]domain.Settlement, error) {
	return f.settlements, nil
}

func (f *fakeSettlementRepo) Delete(_ context.Context, id uint) error {
	f.deletedID = id
	return nil
}

func newTestBalanceService(expenses []domain.Expense, settlements []domain.Settlement) (BalanceService, *fakeSettlementRepo) {
	settlementRepo := &fakeSettlementRepo{settlements: settlements}
	svc := NewBalanceService(
		&fakeExpenseRepo{expenses: expenses},
		&fakeGroupRepo{group: &domain.Group{ID: 1, Members: []domain.User{{ID: 1}, {ID: 2}, {ID: 3}}}},
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

	debts, err := svc.CalculateGroupDebts(context.Background(), 1)
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

	debts, err := svc.CalculateGroupDebts(context.Background(), 1)
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

	debts, err := svc.CalculateGroupDebts(context.Background(), 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(debts) != 0 {
		t.Errorf("expected no remaining debts, got %+v", debts)
	}
}

func TestSettleDebt_RejectsNonPositiveAmount(t *testing.T) {
	svc, _ := newTestBalanceService(nil, nil)

	if _, err := svc.SettleDebt(context.Background(), 1, 2, 1, 0); err == nil {
		t.Error("expected error for zero amount")
	}
	if _, err := svc.SettleDebt(context.Background(), 1, 2, 1, -10); err == nil {
		t.Error("expected error for negative amount")
	}
}

func TestSettleDebt_RejectsSameUser(t *testing.T) {
	svc, _ := newTestBalanceService(nil, nil)

	if _, err := svc.SettleDebt(context.Background(), 1, 1, 1, 10); err == nil {
		t.Error("expected error when from_user_id equals to_user_id")
	}
}

func TestSettleDebt_PersistsSettlement(t *testing.T) {
	// User 1 paid 30, split so user 2 owes the full 30 back.
	expenses := []domain.Expense{
		{
			PaidByID: 1,
			Amount:   30,
			Splits: []domain.ExpenseSplit{
				{UserID: 1, Amount: 0},
				{UserID: 2, Amount: 30},
			},
		},
	}
	svc, settlementRepo := newTestBalanceService(expenses, nil)

	settlement, err := svc.SettleDebt(context.Background(), 1, 2, 1, 30)
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

func TestSettleDebt_RejectsAmountExceedingWhatIsOwed(t *testing.T) {
	// User 2 owes user 1 exactly 30 — trying to settle 31 should fail rather
	// than being recorded and skewing the ledger in the settling party's favor.
	expenses := []domain.Expense{
		{
			PaidByID: 1,
			Amount:   30,
			Splits: []domain.ExpenseSplit{
				{UserID: 1, Amount: 0},
				{UserID: 2, Amount: 30},
			},
		},
	}
	svc, settlementRepo := newTestBalanceService(expenses, nil)

	if _, err := svc.SettleDebt(context.Background(), 1, 2, 1, 31); err == nil {
		t.Error("expected error when the settlement amount exceeds what is owed")
	}
	if len(settlementRepo.created) != 0 {
		t.Errorf("expected no settlement to be persisted, got %d", len(settlementRepo.created))
	}
}

func TestSettleDebt_RejectsWhenNothingIsOwedInThatDirection(t *testing.T) {
	// User 1 owes user 2, not the other way around — settling 2->1 shouldn't
	// be possible just because the two are members with some balance.
	expenses := []domain.Expense{
		{
			PaidByID: 2,
			Amount:   30,
			Splits: []domain.ExpenseSplit{
				{UserID: 1, Amount: 30},
				{UserID: 2, Amount: 0},
			},
		},
	}
	svc, settlementRepo := newTestBalanceService(expenses, nil)

	if _, err := svc.SettleDebt(context.Background(), 1, 2, 1, 10); err == nil {
		t.Error("expected error when nothing is owed from_user_id -> to_user_id")
	}
	if len(settlementRepo.created) != 0 {
		t.Errorf("expected no settlement to be persisted, got %d", len(settlementRepo.created))
	}
}

func TestSettleDebt_RejectsNonMembers(t *testing.T) {
	svc, settlementRepo := newTestBalanceService(nil, nil)

	// User 99 is not a member of the group (members are 1, 2, 3).
	if _, err := svc.SettleDebt(context.Background(), 1, 99, 1, 10); err == nil {
		t.Error("expected error when a party is not a group member")
	}
	if len(settlementRepo.created) != 0 {
		t.Errorf("expected no settlement to be persisted, got %d", len(settlementRepo.created))
	}
}

func TestSettleDebt_GroupNotFound(t *testing.T) {
	settlementRepo := &fakeSettlementRepo{}
	svc := NewBalanceService(
		&fakeExpenseRepo{},
		&fakeGroupRepo{group: nil},
		settlementRepo,
	)

	if _, err := svc.SettleDebt(context.Background(), 1, 2, 1, 30); err == nil {
		t.Error("expected error when group does not exist")
	}
}

func TestDeleteSettlement_AllowsFromUserToDelete(t *testing.T) {
	settlements := []domain.Settlement{{ID: 9, GroupID: 1, FromUserID: 2, ToUserID: 1, Amount: 30}}
	svc, repo := newTestBalanceService(nil, settlements)

	if err := svc.DeleteSettlement(context.Background(), 9, 2); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if repo.deletedID != 9 {
		t.Errorf("expected Delete to be called with id 9, got %d", repo.deletedID)
	}
}

func TestDeleteSettlement_AllowsToUserToDelete(t *testing.T) {
	settlements := []domain.Settlement{{ID: 9, GroupID: 1, FromUserID: 2, ToUserID: 1, Amount: 30}}
	svc, _ := newTestBalanceService(nil, settlements)

	if err := svc.DeleteSettlement(context.Background(), 9, 1); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDeleteSettlement_RejectsBystander(t *testing.T) {
	settlements := []domain.Settlement{{ID: 9, GroupID: 1, FromUserID: 2, ToUserID: 1, Amount: 30}}
	svc, repo := newTestBalanceService(nil, settlements)

	err := svc.DeleteSettlement(context.Background(), 9, 3)
	if !errors.Is(err, ErrNotSettlementParty) {
		t.Errorf("expected ErrNotSettlementParty, got %v", err)
	}
	if repo.deletedID != 0 {
		t.Error("expected Delete not to be called")
	}
}

func TestDeleteSettlement_RejectsUnknownSettlement(t *testing.T) {
	svc, _ := newTestBalanceService(nil, nil)

	err := svc.DeleteSettlement(context.Background(), 999, 1)
	if !errors.Is(err, ErrSettlementNotFound) {
		t.Errorf("expected ErrSettlementNotFound, got %v", err)
	}
}

func TestGetSettlement_ReturnsAnyGroupMemberTheSettlement(t *testing.T) {
	settlements := []domain.Settlement{{ID: 9, GroupID: 1, FromUserID: 2, ToUserID: 1, Amount: 30}}
	svc, _ := newTestBalanceService(nil, settlements)

	settlement, err := svc.GetSettlement(context.Background(), 9)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if settlement.ID != 9 {
		t.Errorf("expected settlement 9, got %+v", settlement)
	}
}

func TestGetSettlement_RejectsUnknownSettlement(t *testing.T) {
	svc, _ := newTestBalanceService(nil, nil)

	_, err := svc.GetSettlement(context.Background(), 999)
	if !errors.Is(err, ErrSettlementNotFound) {
		t.Errorf("expected ErrSettlementNotFound, got %v", err)
	}
}
