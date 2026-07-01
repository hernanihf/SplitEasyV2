package service

import (
	"context"
	"errors"
	"sort"
	"spliteasy/internal/domain"
	"spliteasy/internal/repository"
)

// Sentinel errors that handlers map to HTTP status codes via errors.Is.
var (
	ErrSettlementNotFound = errors.New("settlement not found")
	ErrNotSettlementParty = errors.New("you must be a party to the settlement")
)

type BalanceService interface {
	CalculateGroupDebts(ctx context.Context, groupID uint) ([]domain.Debt, error)
	SettleDebt(ctx context.Context, groupID, fromUserID, toUserID uint, amount int64) (*domain.Settlement, error)
	ListSettlements(ctx context.Context, groupID uint) ([]domain.Settlement, error)
	// DeleteSettlement soft-deletes a settlement. callerID must be the
	// settlement's from_user_id or to_user_id.
	DeleteSettlement(ctx context.Context, settlementID, callerID uint) error
}

type balanceService struct {
	expenseRepo    repository.ExpenseRepository
	groupRepo      repository.GroupRepository
	settlementRepo repository.SettlementRepository
}

func NewBalanceService(
	expenseRepo repository.ExpenseRepository,
	groupRepo repository.GroupRepository,
	settlementRepo repository.SettlementRepository,
) BalanceService {
	return &balanceService{expenseRepo, groupRepo, settlementRepo}
}

func (s *balanceService) CalculateGroupDebts(ctx context.Context, groupID uint) ([]domain.Debt, error) {
	_, err := s.groupRepo.GetByID(ctx, groupID)
	if err != nil {
		return nil, errors.New("group not found")
	}

	expenses, err := s.expenseRepo.GetByGroupID(ctx, groupID)
	if err != nil {
		return nil, err
	}

	settlements, err := s.settlementRepo.GetByGroupID(ctx, groupID)
	if err != nil {
		return nil, err
	}

	// 1. Calculate net balances for each user (in cents)
	balancesMap := make(map[uint]int64)

	for _, exp := range expenses {
		// Payer gets positive balance for the amount they paid
		balancesMap[exp.PaidByID] += exp.Amount

		// For each split, subtract their share (they owe this amount)
		for _, split := range exp.Splits {
			balancesMap[split.UserID] -= split.Amount
		}
	}

	// Settlements reduce what the payer still owes and what the receiver is still owed,
	// regardless of who the outstanding debt ends up being matched against below.
	for _, settlement := range settlements {
		balancesMap[settlement.FromUserID] += settlement.Amount
		balancesMap[settlement.ToUserID] -= settlement.Amount
	}

	// 2. Separate into debtors and creditors
	var debtors []domain.UserBalance
	var creditors []domain.UserBalance

	for userID, amount := range balancesMap {
		if amount < 0 {
			debtors = append(debtors, domain.UserBalance{UserID: userID, Amount: amount})
		} else if amount > 0 {
			creditors = append(creditors, domain.UserBalance{UserID: userID, Amount: amount})
		}
	}

	// Map iteration order is randomised in Go, so sort both sides by UserID to
	// keep the greedy pairing (and thus "who owes whom") stable across requests.
	sort.Slice(debtors, func(i, j int) bool { return debtors[i].UserID < debtors[j].UserID })
	sort.Slice(creditors, func(i, j int) bool { return creditors[i].UserID < creditors[j].UserID })

	// 3. Resolve debts (Greedy algorithm)
	var debts []domain.Debt
	i, j := 0, 0

	for i < len(debtors) && j < len(creditors) {
		debtor := &debtors[i]
		creditor := &creditors[j]

		// Debt amount is negative, so we take its absolute value.
		debtAmount := -debtor.Amount
		creditAmount := creditor.Amount

		settleAmount := debtAmount
		if creditAmount < settleAmount {
			settleAmount = creditAmount
		}

		debts = append(debts, domain.Debt{
			FromUserID: debtor.UserID,
			ToUserID:   creditor.UserID,
			Amount:     settleAmount,
		})

		// Adjust balances
		debtor.Amount += settleAmount
		creditor.Amount -= settleAmount

		// Move indices if balances are settled
		if debtor.Amount == 0 {
			i++
		}
		if creditor.Amount == 0 {
			j++
		}
	}

	// If no debts, return empty list instead of nil for clean JSON output
	if debts == nil {
		debts = []domain.Debt{}
	}

	return debts, nil
}

func (s *balanceService) SettleDebt(ctx context.Context, groupID, fromUserID, toUserID uint, amount int64) (*domain.Settlement, error) {
	if amount <= 0 {
		return nil, errors.New("settlement amount must be positive")
	}
	if fromUserID == toUserID {
		return nil, errors.New("from_user_id and to_user_id must differ")
	}

	group, err := s.groupRepo.GetByID(ctx, groupID)
	if err != nil {
		return nil, errors.New("group not found")
	}
	if !isMember(group, fromUserID) || !isMember(group, toUserID) {
		return nil, errors.New("both users must be members of the group")
	}

	// Cap the settlement at what's actually owed in this direction right now
	// — the same figure the Balances tab shows. Without this, a party to the
	// debt (the only people who can call this at all, per the check above)
	// could log an arbitrarily large "payment" and skew the ledger in their
	// own favor rather than just recording a real payment.
	debts, err := s.CalculateGroupDebts(ctx, groupID)
	if err != nil {
		return nil, err
	}
	var owed int64
	for _, d := range debts {
		if d.FromUserID == fromUserID && d.ToUserID == toUserID {
			owed = d.Amount
			break
		}
	}
	if amount > owed {
		return nil, errors.New("settlement amount exceeds what is currently owed")
	}

	settlement := &domain.Settlement{
		GroupID:    groupID,
		FromUserID: fromUserID,
		ToUserID:   toUserID,
		Amount:     amount,
	}

	if err := s.settlementRepo.Create(ctx, settlement); err != nil {
		return nil, err
	}

	return settlement, nil
}

// ListSettlements returns every recorded payment in the group, for the unified
// history view. Balances are still computed separately from these.
func (s *balanceService) ListSettlements(ctx context.Context, groupID uint) ([]domain.Settlement, error) {
	settlements, err := s.settlementRepo.GetByGroupID(ctx, groupID)
	if err != nil {
		return nil, err
	}
	if settlements == nil {
		settlements = []domain.Settlement{}
	}
	return settlements, nil
}

func (s *balanceService) DeleteSettlement(ctx context.Context, settlementID, callerID uint) error {
	settlement, err := s.settlementRepo.GetByID(ctx, settlementID)
	if err != nil {
		return ErrSettlementNotFound
	}
	if settlement.FromUserID != callerID && settlement.ToUserID != callerID {
		return ErrNotSettlementParty
	}
	return s.settlementRepo.Delete(ctx, settlementID)
}
