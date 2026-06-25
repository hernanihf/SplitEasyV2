package service

import (
	"errors"
	"math"
	"sort"
	"spliteasy/internal/domain"
	"spliteasy/internal/repository"
)

type BalanceService interface {
	CalculateGroupDebts(groupID uint) ([]domain.Debt, error)
	SettleDebt(groupID, fromUserID, toUserID uint, amount float64) (*domain.Settlement, error)
	ListSettlements(groupID uint) ([]domain.Settlement, error)
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

func (s *balanceService) CalculateGroupDebts(groupID uint) ([]domain.Debt, error) {
	_, err := s.groupRepo.GetByID(groupID)
	if err != nil {
		return nil, errors.New("group not found")
	}

	expenses, err := s.expenseRepo.GetByGroupID(groupID)
	if err != nil {
		return nil, err
	}

	settlements, err := s.settlementRepo.GetByGroupID(groupID)
	if err != nil {
		return nil, err
	}

	// 1. Calculate net balances for each user
	balancesMap := make(map[uint]float64)

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
		amount = math.Round(amount*100) / 100 // Avoid floating point precision issues
		if amount < -0.01 {
			debtors = append(debtors, domain.UserBalance{UserID: userID, Amount: amount})
		} else if amount > 0.01 {
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

		// Debt amount is negative, so we take absolute value
		debtAmount := math.Abs(debtor.Amount)
		creditAmount := creditor.Amount

		settleAmount := math.Min(debtAmount, creditAmount)
		settleAmount = math.Round(settleAmount*100) / 100

		debts = append(debts, domain.Debt{
			FromUserID: debtor.UserID,
			ToUserID:   creditor.UserID,
			Amount:     settleAmount,
		})

		// Adjust balances
		debtor.Amount += settleAmount
		creditor.Amount -= settleAmount

		// Move indices if balances are settled
		if math.Abs(debtor.Amount) < 0.01 {
			i++
		}
		if creditor.Amount < 0.01 {
			j++
		}
	}

	// If no debts, return empty list instead of nil for clean JSON output
	if debts == nil {
		debts = []domain.Debt{}
	}

	return debts, nil
}

func (s *balanceService) SettleDebt(groupID, fromUserID, toUserID uint, amount float64) (*domain.Settlement, error) {
	if amount <= 0 {
		return nil, errors.New("settlement amount must be positive")
	}
	if fromUserID == toUserID {
		return nil, errors.New("from_user_id and to_user_id must differ")
	}

	group, err := s.groupRepo.GetByID(groupID)
	if err != nil {
		return nil, errors.New("group not found")
	}
	if !isMember(group, fromUserID) || !isMember(group, toUserID) {
		return nil, errors.New("both users must be members of the group")
	}

	settlement := &domain.Settlement{
		GroupID:    groupID,
		FromUserID: fromUserID,
		ToUserID:   toUserID,
		Amount:     amount,
	}

	if err := s.settlementRepo.Create(settlement); err != nil {
		return nil, err
	}

	return settlement, nil
}

// ListSettlements returns every recorded payment in the group, for the unified
// history view. Balances are still computed separately from these.
func (s *balanceService) ListSettlements(groupID uint) ([]domain.Settlement, error) {
	settlements, err := s.settlementRepo.GetByGroupID(groupID)
	if err != nil {
		return nil, err
	}
	if settlements == nil {
		settlements = []domain.Settlement{}
	}
	return settlements, nil
}
