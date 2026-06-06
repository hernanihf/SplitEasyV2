package service

import (
	"errors"
	"math"
	"spliteasy/internal/domain"
	"spliteasy/internal/repository"
)

type ExpenseService interface {
	AddExpense(groupID, paidByID uint, description string, amount float64) (*domain.Expense, error)
	GetGroupExpenses(groupID uint) ([]domain.Expense, error)
}

type expenseService struct {
	expenseRepo repository.ExpenseRepository
	groupRepo   repository.GroupRepository
}

func NewExpenseService(expenseRepo repository.ExpenseRepository, groupRepo repository.GroupRepository) ExpenseService {
	return &expenseService{expenseRepo, groupRepo}
}

func (s *expenseService) AddExpense(groupID, paidByID uint, description string, amount float64) (*domain.Expense, error) {
	if amount <= 0 {
		return nil, errors.New("amount must be greater than zero")
	}

	group, err := s.groupRepo.GetByID(groupID)
	if err != nil {
		return nil, errors.New("group not found")
	}

	// Verify paidBy is a member
	isMember := false
	for _, member := range group.Members {
		if member.ID == paidByID {
			isMember = true
			break
		}
	}
	if !isMember {
		return nil, errors.New("payer is not a member of the group")
	}

	numMembers := len(group.Members)
	if numMembers == 0 {
		return nil, errors.New("group has no members to split the expense")
	}

	// Calculate split equally
	splitAmount := math.Round((amount/float64(numMembers))*100) / 100

	expense := &domain.Expense{
		GroupID:     groupID,
		PaidByID:    paidByID,
		Description: description,
		Amount:      amount,
	}

	var splits []domain.ExpenseSplit
	for _, member := range group.Members {
		splits = append(splits, domain.ExpenseSplit{
			UserID: member.ID,
			Amount: splitAmount,
		})
	}

	err = s.expenseRepo.CreateWithSplits(expense, splits)
	if err != nil {
		return nil, err
	}

	expense.Splits = splits
	return expense, nil
}

func (s *expenseService) GetGroupExpenses(groupID uint) ([]domain.Expense, error) {
	return s.expenseRepo.GetByGroupID(groupID)
}
