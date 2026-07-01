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

// Sentinel errors that handlers map to HTTP status codes via errors.Is.
var (
	ErrExpenseNotFound = errors.New("expense not found")
	ErrNotExpenseParty = errors.New("you must be the payer or one of the split participants")
)

type ExpenseService interface {
	AddExpense(ctx context.Context, groupID, paidByID uint, description string, amount int64, method SplitMethod, splitInputs []SplitInput, items []ItemInput) (*domain.Expense, error)
	// UpdateExpense replaces an existing expense's fields, split, and items.
	// callerID must be the current payer or a current split participant —
	// checked against the expense as it exists now, before any of these
	// changes are applied.
	UpdateExpense(ctx context.Context, expenseID, callerID, paidByID uint, description string, amount int64, method SplitMethod, splitInputs []SplitInput, items []ItemInput) (*domain.Expense, error)
	// DeleteExpense soft-deletes an expense. Same authorization as Update.
	DeleteExpense(ctx context.Context, expenseID, callerID uint) error
	// GetExpense fetches a single expense by id. Unlike Update/Delete, there's
	// no payer-or-participant check here — the caller only needs to be a
	// member of the expense's group (enforced by the handler, which needs
	// the expense's GroupID from this call before it can check that).
	GetExpense(ctx context.Context, expenseID uint) (*domain.Expense, error)
	GetGroupExpenses(ctx context.Context, groupID uint) ([]domain.Expense, error)
}

type expenseService struct {
	expenseRepo repository.ExpenseRepository
	groupRepo   repository.GroupRepository
}

func NewExpenseService(expenseRepo repository.ExpenseRepository, groupRepo repository.GroupRepository) ExpenseService {
	return &expenseService{expenseRepo, groupRepo}
}

// resolveSplitsAndItems validates paidByID/splitInputs/items against the
// group's membership and turns them into persistable splits/items. Shared by
// AddExpense and UpdateExpense so the two can't drift apart on validation.
func resolveSplitsAndItems(amount int64, method SplitMethod, paidByID uint, splitInputs []SplitInput, items []ItemInput, group *domain.Group) ([]domain.ExpenseSplit, []domain.ExpenseItem, error) {
	if amount <= 0 {
		return nil, nil, errors.New("amount must be greater than zero")
	}

	memberIDs := make(map[uint]bool, len(group.Members))
	for _, member := range group.Members {
		memberIDs[member.ID] = true
	}

	if !memberIDs[paidByID] {
		return nil, nil, errors.New("payer is not a member of the group")
	}

	for _, input := range splitInputs {
		if !memberIDs[input.UserID] {
			return nil, nil, errors.New("split includes a user who is not a member of the group")
		}
	}

	splitAmounts, err := calculateSplitAmounts(method, amount, splitInputs, group.Members)
	if err != nil {
		return nil, nil, err
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
				return nil, nil, errors.New("item assigned to a user who is not a member of the group")
			}
			users = append(users, domain.User{ID: uid})
		}
		domainItems = append(domainItems, domain.ExpenseItem{
			Description: item.Description,
			Amount:      item.Amount,
			Users:       users,
		})
	}

	return splits, domainItems, nil
}

// isPayerOrSplitParticipant reports whether userID has a real stake in the
// expense as it currently exists — either they paid it or they're one of the
// people splitting it. Used to gate Update/Delete: without it, anyone in the
// group could edit or delete an expense they have nothing to do with.
func isPayerOrSplitParticipant(userID uint, expense *domain.Expense) bool {
	if expense.PaidByID == userID {
		return true
	}
	for _, split := range expense.Splits {
		if split.UserID == userID {
			return true
		}
	}
	return false
}

func (s *expenseService) AddExpense(ctx context.Context, groupID, paidByID uint, description string, amount int64, method SplitMethod, splitInputs []SplitInput, items []ItemInput) (*domain.Expense, error) {
	group, err := s.groupRepo.GetByID(ctx, groupID)
	if err != nil {
		return nil, errors.New("group not found")
	}

	splits, domainItems, err := resolveSplitsAndItems(amount, method, paidByID, splitInputs, items, group)
	if err != nil {
		return nil, err
	}

	expense := &domain.Expense{
		GroupID:     groupID,
		PaidByID:    paidByID,
		Description: description,
		Amount:      amount,
	}

	if err := s.expenseRepo.CreateWithSplits(ctx, expense, splits, domainItems); err != nil {
		return nil, err
	}

	expense.Splits = splits
	expense.Items = domainItems
	return expense, nil
}

func (s *expenseService) UpdateExpense(ctx context.Context, expenseID, callerID, paidByID uint, description string, amount int64, method SplitMethod, splitInputs []SplitInput, items []ItemInput) (*domain.Expense, error) {
	existing, err := s.expenseRepo.GetByID(ctx, expenseID)
	if err != nil {
		return nil, ErrExpenseNotFound
	}
	// Checked against the expense as it exists now (its current payer and
	// splits), before any of the requested changes are applied — otherwise
	// editing would be an easier way to do exactly what AddExpense's
	// authorization check blocks: touch an expense you have nothing to do
	// with.
	if !isPayerOrSplitParticipant(callerID, existing) {
		return nil, ErrNotExpenseParty
	}

	group, err := s.groupRepo.GetByID(ctx, existing.GroupID)
	if err != nil {
		return nil, errors.New("group not found")
	}

	// The new payer doesn't have to be the caller — same "log for a
	// roommate" flow AddExpense allows — just a group member, enforced by
	// resolveSplitsAndItems below.
	splits, domainItems, err := resolveSplitsAndItems(amount, method, paidByID, splitInputs, items, group)
	if err != nil {
		return nil, err
	}

	existing.PaidByID = paidByID
	existing.Description = description
	existing.Amount = amount

	if err := s.expenseRepo.UpdateWithSplits(ctx, existing, splits, domainItems); err != nil {
		return nil, err
	}

	existing.Splits = splits
	existing.Items = domainItems
	return existing, nil
}

func (s *expenseService) DeleteExpense(ctx context.Context, expenseID, callerID uint) error {
	existing, err := s.expenseRepo.GetByID(ctx, expenseID)
	if err != nil {
		return ErrExpenseNotFound
	}
	if !isPayerOrSplitParticipant(callerID, existing) {
		return ErrNotExpenseParty
	}
	return s.expenseRepo.Delete(ctx, expenseID)
}

func (s *expenseService) GetExpense(ctx context.Context, expenseID uint) (*domain.Expense, error) {
	expense, err := s.expenseRepo.GetByID(ctx, expenseID)
	if err != nil {
		return nil, ErrExpenseNotFound
	}
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
