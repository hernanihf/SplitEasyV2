package service

import (
	"context"
	"errors"
	"math"
	"sort"

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
	// SplitFixed assigns an exact amount (in cents) per member; amounts must
	// add up to the expense total.
	SplitFixed SplitMethod = "fixed"
	// SplitShares divides the amount proportionally to a weight/unit count
	// per member (e.g. 2 units of bread vs 4 units of bread).
	SplitShares SplitMethod = "shares"
)

// SplitInput represents one member's share for non-equal split methods, or
// simply which member to include for an equal split among a subset. Value is
// interpreted per method: cents for "fixed", a percentage for "percentage", a
// weight for "shares", and unused for "equal".
type SplitInput struct {
	UserID uint
	Value  float64
}

// ItemInput is a single line item of an itemized expense, assigned to one or
// more members. Items are persisted for display; they don't drive the balances
// (the computed Splits do). Amount is in cents.
type ItemInput struct {
	Description string
	Amount      int64
	UserIDs     []uint
}

type ExpenseService interface {
	AddExpense(ctx context.Context, groupID, paidByID uint, description string, amount int64, method SplitMethod, splitInputs []SplitInput, items []ItemInput) (*domain.Expense, error)
	GetGroupExpenses(ctx context.Context, groupID uint) ([]domain.Expense, error)
}

type expenseService struct {
	expenseRepo repository.ExpenseRepository
	groupRepo   repository.GroupRepository
}

func NewExpenseService(expenseRepo repository.ExpenseRepository, groupRepo repository.GroupRepository) ExpenseService {
	return &expenseService{expenseRepo, groupRepo}
}

func (s *expenseService) AddExpense(ctx context.Context, groupID, paidByID uint, description string, amount int64, method SplitMethod, splitInputs []SplitInput, items []ItemInput) (*domain.Expense, error) {
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

	// Build line items (optional). Each item is assigned to members; those
	// users must belong to the group.
	var domainItems []domain.ExpenseItem
	for _, item := range items {
		users := make([]domain.User, 0, len(item.UserIDs))
		for _, uid := range item.UserIDs {
			if !memberIDs[uid] {
				return nil, errors.New("item assigned to a user who is not a member of the group")
			}
			users = append(users, domain.User{ID: uid})
		}
		domainItems = append(domainItems, domain.ExpenseItem{
			Description: item.Description,
			Amount:      item.Amount,
			Users:       users,
		})
	}

	if err := s.expenseRepo.CreateWithSplits(ctx, expense, splits, domainItems); err != nil {
		return nil, err
	}

	expense.Splits = splits
	expense.Items = domainItems
	return expense, nil
}

// calculateSplitAmounts resolves how many cents each member owes for the given
// method. Map order is irrelevant; callers only care about user->cents.
func calculateSplitAmounts(method SplitMethod, amount int64, splitInputs []SplitInput, groupMembers []domain.User) (map[uint]int64, error) {
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

// distributeProportionally splits total cents among userIDs proportional to the
// given weights, using the largest-remainder method so the parts add up exactly
// to total (no cent is lost or invented). Ties are broken by user id so the
// result is deterministic.
func distributeProportionally(total int64, userIDs []uint, weights []float64) map[uint]int64 {
	result := make(map[uint]int64, len(userIDs))

	var weightSum float64
	for _, w := range weights {
		weightSum += w
	}
	if weightSum <= 0 {
		return result
	}

	type remainder struct {
		userID uint
		frac   float64
	}
	rems := make([]remainder, len(userIDs))

	var assigned int64
	for i, uid := range userIDs {
		exact := float64(total) * weights[i] / weightSum
		floor := int64(math.Floor(exact))
		result[uid] = floor
		assigned += floor
		rems[i] = remainder{userID: uid, frac: exact - float64(floor)}
	}

	// Hand out the leftover cents to the largest fractional remainders.
	leftover := total - assigned
	sort.Slice(rems, func(i, j int) bool {
		if rems[i].frac != rems[j].frac {
			return rems[i].frac > rems[j].frac
		}
		return rems[i].userID < rems[j].userID
	})
	for i := int64(0); i < leftover && int(i) < len(rems); i++ {
		result[rems[i].userID]++
	}

	return result
}

func splitEqual(amount int64, splitInputs []SplitInput, groupMembers []domain.User) (map[uint]int64, error) {
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

	weights := make([]float64, len(userIDs))
	for i := range weights {
		weights[i] = 1
	}
	return distributeProportionally(amount, userIDs, weights), nil
}

func splitByPercentage(amount int64, splitInputs []SplitInput) (map[uint]int64, error) {
	if len(splitInputs) == 0 {
		return nil, errors.New("percentage split requires at least one member")
	}

	var totalPercentage float64
	userIDs := make([]uint, len(splitInputs))
	weights := make([]float64, len(splitInputs))
	for i, input := range splitInputs {
		if input.Value < 0 {
			return nil, errors.New("percentages must not be negative")
		}
		totalPercentage += input.Value
		userIDs[i] = input.UserID
		weights[i] = input.Value
	}

	if math.Abs(totalPercentage-100) > 0.01 {
		return nil, errors.New("percentages must add up to 100")
	}
	return distributeProportionally(amount, userIDs, weights), nil
}

func splitByFixedAmount(amount int64, splitInputs []SplitInput) (map[uint]int64, error) {
	if len(splitInputs) == 0 {
		return nil, errors.New("fixed split requires at least one member")
	}

	var total int64
	result := make(map[uint]int64, len(splitInputs))
	for _, input := range splitInputs {
		if input.Value < 0 {
			return nil, errors.New("fixed amounts must not be negative")
		}
		cents := int64(math.Round(input.Value))
		total += cents
		result[input.UserID] = cents
	}

	if total != amount {
		return nil, errors.New("fixed amounts must add up to the expense total")
	}
	return result, nil
}

func splitByShares(amount int64, splitInputs []SplitInput) (map[uint]int64, error) {
	if len(splitInputs) == 0 {
		return nil, errors.New("shares split requires at least one member")
	}

	userIDs := make([]uint, len(splitInputs))
	weights := make([]float64, len(splitInputs))
	for i, input := range splitInputs {
		if input.Value <= 0 {
			return nil, errors.New("shares must be greater than zero")
		}
		userIDs[i] = input.UserID
		weights[i] = input.Value
	}

	return distributeProportionally(amount, userIDs, weights), nil
}

func (s *expenseService) GetGroupExpenses(ctx context.Context, groupID uint) ([]domain.Expense, error) {
	return s.expenseRepo.GetByGroupID(ctx, groupID)
}
