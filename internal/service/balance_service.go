package service

import (
	"errors"
	"math"
	"spliteasy/internal/domain"
	"spliteasy/internal/repository"
)

type BalanceService interface {
	CalculateGroupDebts(groupID uint) ([]domain.Debt, error)
}

type balanceService struct {
	expenseRepo repository.ExpenseRepository
	groupRepo   repository.GroupRepository
}

func NewBalanceService(expenseRepo repository.ExpenseRepository, groupRepo repository.GroupRepository) BalanceService {
	return &balanceService{expenseRepo, groupRepo}
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
