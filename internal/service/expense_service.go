package service

import (
	"context"
	"errors"
	"math"
	"spliteasy/internal/domain"
	"spliteasy/internal/repository"
)

type SplitMethod string

const (
	// SplitEqual divides the amount evenly among the given members (or all
	// group members if none are given).
	SplitEqual SplitMethod = "equal"
	// SplitPercentage divides the amount according to a percentage (0-100)
	// per member; percentages must add up to 100.
	SplitPercentage SplitMethod = "percentage"
	// SplitFixed assigns an exact amount per member; amounts must add up to
	// the expense total.
	SplitFixed SplitMethod = "fixed"
	// SplitShares divides the amount proportionally to a weight/unit count
	// per member (e.g. 2 units of bread vs 4 units of bread).
	SplitShares SplitMethod = "shares"
)

// SplitInput represents one member's share for non-equal split methods, or
// simply which member to include for an equal split among a subset.
type SplitInput struct {
	UserID uint
	Value  float64
}

type ExpenseService interface {
	AddExpense(ctx context.Context, groupID, paidByID uint, description string, amount float64, method SplitMethod, splitInputs []SplitInput) (*domain.Expense, error)
	GetGroupExpenses(ctx context.Context, groupID uint) ([]domain.Expense, error)
}

type expenseService struct {
	expenseRepo repository.ExpenseRepository
	groupRepo   repository.GroupRepository
}

func NewExpenseService(expenseRepo repository.ExpenseRepository, groupRepo repository.GroupRepository) ExpenseService {
	return &expenseService{expenseRepo, groupRepo}
}

func (s *expenseService) AddExpense(ctx context.Context, groupID, paidByID uint, description string, amount float64, method SplitMethod, splitInputs []SplitInput) (*domain.Expense, error) {
	if amount <= 0 {
		return nil, errors.New("amount must be greater than zero")
	}

	group, err := s.groupRepo.GetByID(ctx, groupID)
	if err != nil {
		return nil, errors.New("group not found")
	}

	memberIDs := make(map[uint]bool, len(group.Members))
	for _, member := range group.Members {
		memberIDs[member.ID] = true
	}

	if !memberIDs[paidByID] {
		return nil, errors.New("payer is not a member of the group")
	}

	for _, input := range splitInputs {
		if !memberIDs[input.UserID] {
			return nil, errors.New("split includes a user who is not a member of the group")
		}
	}

	splitAmounts, err := calculateSplitAmounts(method, amount, splitInputs, group.Members)
	if err != nil {
		return nil, err
	}

	expense := &domain.Expense{
		GroupID:     groupID,
		PaidByID:    paidByID,
		Description: description,
		Amount:      amount,
	}

	var splits []domain.ExpenseSplit
	for userID, splitAmount := range splitAmounts {
		splits = append(splits, domain.ExpenseSplit{
			UserID: userID,
			Amount: splitAmount,
		})
	}

	if err := s.expenseRepo.CreateWithSplits(ctx, expense, splits); err != nil {
		return nil, err
	}

	expense.Splits = splits
	return expense, nil
}

// calculateSplitAmounts resolves how much each member owes for the given
// method. Map order is irrelevant; callers only care about user->amount.
func calculateSplitAmounts(method SplitMethod, amount float64, splitInputs []SplitInput, groupMembers []domain.User) (map[uint]float64, error) {
	switch method {
	case "", SplitEqual:
		return splitEqual(amount, splitInputs, groupMembers)
	case SplitPercentage:
		return splitByPercentage(amount, splitInputs)
	case SplitFixed:
		return splitByFixedAmount(amount, splitInputs)
	case SplitShares:
		return splitByShares(amount, splitInputs)
	default:
		return nil, errors.New("unsupported split method")
	}
}

func splitEqual(amount float64, splitInputs []SplitInput, groupMembers []domain.User) (map[uint]float64, error) {
	var userIDs []uint
	if len(splitInputs) > 0 {
		for _, input := range splitInputs {
			userIDs = append(userIDs, input.UserID)
		}
	} else {
		for _, member := range groupMembers {
			userIDs = append(userIDs, member.ID)
		}
	}

	if len(userIDs) == 0 {
		return nil, errors.New("no members to split the expense among")
	}

	splitAmount := roundCents(amount / float64(len(userIDs)))
	result := make(map[uint]float64, len(userIDs))
	for _, userID := range userIDs {
		result[userID] = splitAmount
	}
	return result, nil
}

func splitByPercentage(amount float64, splitInputs []SplitInput) (map[uint]float64, error) {
	if len(splitInputs) == 0 {
		return nil, errors.New("percentage split requires at least one member")
	}

	var totalPercentage float64
	result := make(map[uint]float64, len(splitInputs))
	for _, input := range splitInputs {
		totalPercentage += input.Value
		result[input.UserID] = roundCents(amount * input.Value / 100)
	}

	if math.Abs(totalPercentage-100) > 0.01 {
		return nil, errors.New("percentages must add up to 100")
	}
	return result, nil
}

func splitByFixedAmount(amount float64, splitInputs []SplitInput) (map[uint]float64, error) {
	if len(splitInputs) == 0 {
		return nil, errors.New("fixed split requires at least one member")
	}

	var total float64
	result := make(map[uint]float64, len(splitInputs))
	for _, input := range splitInputs {
		total += input.Value
		result[input.UserID] = roundCents(input.Value)
	}

	if math.Abs(total-amount) > 0.01 {
		return nil, errors.New("fixed amounts must add up to the expense total")
	}
	return result, nil
}

func splitByShares(amount float64, splitInputs []SplitInput) (map[uint]float64, error) {
	if len(splitInputs) == 0 {
		return nil, errors.New("shares split requires at least one member")
	}

	var totalShares float64
	for _, input := range splitInputs {
		if input.Value <= 0 {
			return nil, errors.New("shares must be greater than zero")
		}
		totalShares += input.Value
	}

	result := make(map[uint]float64, len(splitInputs))
	for _, input := range splitInputs {
		result[input.UserID] = roundCents(amount * input.Value / totalShares)
	}
	return result, nil
}

func roundCents(value float64) float64 {
	return math.Round(value*100) / 100
}

func (s *expenseService) GetGroupExpenses(ctx context.Context, groupID uint) ([]domain.Expense, error) {
	return s.expenseRepo.GetByGroupID(ctx, groupID)
}
