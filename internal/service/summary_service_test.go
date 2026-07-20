package service

import (
	"context"
	"testing"
	"time"

	"spliteasy/internal/domain"

	"gorm.io/gorm"
)

type fakeGroupRepoForSummary struct {
	groups []domain.Group
}

func (f *fakeGroupRepoForSummary) Create(_ context.Context, _ *domain.Group) error { return nil }

func (f *fakeGroupRepoForSummary) GetByID(_ context.Context, id uint) (*domain.Group, error) {
	for _, g := range f.groups {
		if g.ID == id {
			return &g, nil
		}
	}
	return nil, errExpected
}

func (f *fakeGroupRepoForSummary) GetByUserID(_ context.Context, _ uint) ([]domain.Group, error) {
	return f.groups, nil
}

func (f *fakeGroupRepoForSummary) GetByInviteToken(_ context.Context, _ string) (*domain.Group, error) {
	return nil, errExpected
}

func (f *fakeGroupRepoForSummary) AddMember(_ context.Context, _, _ uint) error { return nil }

func (f *fakeGroupRepoForSummary) SetInviteTokenIfEmpty(_ context.Context, _ uint, _ string) error {
	return nil
}

func (f *fakeGroupRepoForSummary) UpdateNameAndEmoji(_ context.Context, _ uint, _, _ *string) error {
	return nil
}

func (f *fakeGroupRepoForSummary) GetExpenseReceiptImagePaths(_ context.Context, _ uint) ([]string, error) {
	return nil, nil
}

func (f *fakeGroupRepoForSummary) Delete(_ context.Context, _ uint) error { return nil }

// fakeExpenseRepoByGroup and fakeSettlementRepoByGroup, unlike the
// same-named fakes in balance_service_test.go, actually filter by group id
// — GetHomeSummary iterates multiple groups per test and needs each one to
// see only its own expenses/settlements.
type fakeExpenseRepoByGroup struct {
	byGroup map[uint][]domain.Expense
}

func (f *fakeExpenseRepoByGroup) CreateWithSplits(_ context.Context, _ *domain.Expense, _ []domain.ExpenseSplit, _ []domain.ExpenseItem) error {
	return nil
}
func (f *fakeExpenseRepoByGroup) UpdateWithSplits(_ context.Context, _ *domain.Expense, _ []domain.ExpenseSplit, _ []domain.ExpenseItem) error {
	return nil
}
func (f *fakeExpenseRepoByGroup) GetByID(_ context.Context, _ uint) (*domain.Expense, error) {
	return nil, errExpected
}
func (f *fakeExpenseRepoByGroup) GetByIDIncludingDeleted(_ context.Context, _ uint) (*domain.Expense, error) {
	return nil, errExpected
}
func (f *fakeExpenseRepoByGroup) GetByGroupID(_ context.Context, groupID uint) ([]domain.Expense, error) {
	return f.byGroup[groupID], nil
}
func (f *fakeExpenseRepoByGroup) GetByGroupIDIncludingDeleted(_ context.Context, groupID uint) ([]domain.Expense, error) {
	return f.byGroup[groupID], nil
}
func (f *fakeExpenseRepoByGroup) Delete(_ context.Context, _, _ uint) error { return nil }
func (f *fakeExpenseRepoByGroup) GetOldSoftDeletedReceiptImagePaths(_ context.Context, _ time.Time) ([]string, error) {
	return nil, nil
}
func (f *fakeExpenseRepoByGroup) PurgeOldSoftDeleted(_ context.Context, _ time.Time) (int64, error) {
	return 0, nil
}

type fakeSettlementRepoByGroup struct {
	byGroup map[uint][]domain.Settlement
}

func (f *fakeSettlementRepoByGroup) Create(_ context.Context, _ *domain.Settlement) error {
	return nil
}
func (f *fakeSettlementRepoByGroup) GetByID(_ context.Context, _ uint) (*domain.Settlement, error) {
	return nil, errExpected
}
func (f *fakeSettlementRepoByGroup) GetByGroupID(_ context.Context, groupID uint) ([]domain.Settlement, error) {
	return f.byGroup[groupID], nil
}
func (f *fakeSettlementRepoByGroup) Delete(_ context.Context, _ uint) error { return nil }

// fakeCommentRepoForSummary indexes comments by their parent id, mirroring
// how the real repository's GetByExpenseIDs/GetBySettlementIDs filter.
type fakeCommentRepoForSummary struct {
	byExpenseID    map[uint][]domain.Comment
	bySettlementID map[uint][]domain.Comment
}

func (f *fakeCommentRepoForSummary) Create(_ context.Context, _ *domain.Comment) error { return nil }
func (f *fakeCommentRepoForSummary) GetByExpenseID(_ context.Context, id uint) ([]domain.Comment, error) {
	return f.byExpenseID[id], nil
}
func (f *fakeCommentRepoForSummary) GetBySettlementID(_ context.Context, id uint) ([]domain.Comment, error) {
	return f.bySettlementID[id], nil
}
func (f *fakeCommentRepoForSummary) GetByExpenseIDs(_ context.Context, ids []uint) ([]domain.Comment, error) {
	var out []domain.Comment
	for _, id := range ids {
		out = append(out, f.byExpenseID[id]...)
	}
	return out, nil
}
func (f *fakeCommentRepoForSummary) GetBySettlementIDs(_ context.Context, ids []uint) ([]domain.Comment, error) {
	var out []domain.Comment
	for _, id := range ids {
		out = append(out, f.bySettlementID[id]...)
	}
	return out, nil
}
func (f *fakeCommentRepoForSummary) GetByID(_ context.Context, _ uint) (*domain.Comment, error) {
	return nil, errExpected
}
func (f *fakeCommentRepoForSummary) Delete(_ context.Context, _ uint) error { return nil }

// fakeUserRepoForSummary is keyed by id so GetUnreadActivityCount can look up
// ActivityLastSeenAt, and UpdateActivityLastSeenAt mutates that same map in
// place so MarkActivitySeen's effect is observable within a test.
type fakeUserRepoForSummary struct {
	usersByID map[uint]*domain.User
}

func (f *fakeUserRepoForSummary) Create(_ context.Context, _ *domain.User) error { return nil }
func (f *fakeUserRepoForSummary) GetByID(_ context.Context, id uint) (*domain.User, error) {
	u, ok := f.usersByID[id]
	if !ok {
		return nil, errExpected
	}
	return u, nil
}
func (f *fakeUserRepoForSummary) GetByEmail(_ context.Context, _ string) (*domain.User, error) {
	return nil, errExpected
}
func (f *fakeUserRepoForSummary) Update(_ context.Context, _ *domain.User) error { return nil }
func (f *fakeUserRepoForSummary) UpdatePushPreferences(_ context.Context, _ uint, _, _, _, _ bool) error {
	return nil
}
func (f *fakeUserRepoForSummary) UpdateActivityLastSeenAt(_ context.Context, userID uint, seenAt time.Time) error {
	if u, ok := f.usersByID[userID]; ok {
		u.ActivityLastSeenAt = seenAt
	}
	return nil
}

func TestGetHomeSummary_BreaksDownByCurrency(t *testing.T) {
	groups := []domain.Group{
		{ID: 1, Name: "USD Trip", Currency: "USD", Members: []domain.User{{ID: 1}, {ID: 2}}},
		{ID: 2, Name: "ARS Asado", Currency: "ARS", Members: []domain.User{{ID: 1}, {ID: 2}}},
	}
	expensesByGroup := map[uint][]domain.Expense{
		// Group 1 (USD): user 1 paid 100, split evenly — user 1 is owed 50.
		1: {{PaidByID: 1, Amount: 100, Splits: []domain.ExpenseSplit{{UserID: 1, Amount: 50}, {UserID: 2, Amount: 50}}}},
		// Group 2 (ARS): user 2 paid 4000, split evenly — user 1 owes 2000.
		2: {{PaidByID: 2, Amount: 4000, Splits: []domain.ExpenseSplit{{UserID: 1, Amount: 2000}, {UserID: 2, Amount: 2000}}}},
	}

	svc := NewSummaryService(
		&fakeGroupRepoForSummary{groups: groups},
		&fakeExpenseRepoByGroup{byGroup: expensesByGroup},
		&fakeSettlementRepoByGroup{},
		&fakeCommentRepoForSummary{},
		&fakeUserRepoForSummary{},
	)

	summary, err := svc.GetHomeSummary(context.Background(), 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(summary.OverallByCurrency) != 2 {
		t.Fatalf("expected 2 currency totals, got %d: %+v", len(summary.OverallByCurrency), summary.OverallByCurrency)
	}

	byCurrency := map[string]domain.OverallBalance{}
	for _, o := range summary.OverallByCurrency {
		byCurrency[o.Currency] = o
	}

	usd := byCurrency["USD"]
	if usd.Net != 50 || usd.Owed != 50 || usd.Owe != 0 {
		t.Errorf("unexpected USD total: %+v", usd)
	}
	ars := byCurrency["ARS"]
	if ars.Net != -2000 || ars.Owed != 0 || ars.Owe != 2000 {
		t.Errorf("unexpected ARS total: %+v", ars)
	}

	if len(summary.Groups) != 2 {
		t.Fatalf("expected 2 group summaries, got %d", len(summary.Groups))
	}
	for _, g := range summary.Groups {
		if g.Currency == "" {
			t.Errorf("expected group summary to carry its currency, got empty for group %d", g.ID)
		}
	}
}

func TestGetActivity_CarriesGroupCurrency(t *testing.T) {
	groups := []domain.Group{
		{ID: 1, Name: "ARS Asado", Currency: "ARS", Members: []domain.User{{ID: 1}}},
	}
	expensesByGroup := map[uint][]domain.Expense{
		1: {{ID: 9, PaidByID: 1, Description: "Carne", Amount: 5000}},
	}

	svc := NewSummaryService(
		&fakeGroupRepoForSummary{groups: groups},
		&fakeExpenseRepoByGroup{byGroup: expensesByGroup},
		&fakeSettlementRepoByGroup{},
		&fakeCommentRepoForSummary{},
		&fakeUserRepoForSummary{},
	)

	events, err := svc.GetActivity(context.Background(), 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].Currency != "ARS" {
		t.Errorf("expected event currency %q, got %q", "ARS", events[0].Currency)
	}
}

func TestGetActivity_FlagsSoftDeletedExpense(t *testing.T) {
	deleter := uint(2)
	groups := []domain.Group{
		{ID: 1, Name: "Asado", Members: []domain.User{{ID: 1}, {ID: 2}}},
	}
	expensesByGroup := map[uint][]domain.Expense{
		1: {{
			ID: 9, PaidByID: 1, Description: "Carne", Amount: 5000,
			DeletedAt:   gorm.DeletedAt{Time: time.Now(), Valid: true},
			DeletedByID: &deleter,
			DeletedBy:   &domain.User{ID: 2, Name: "Bob"},
		}},
	}

	svc := NewSummaryService(
		&fakeGroupRepoForSummary{groups: groups},
		&fakeExpenseRepoByGroup{byGroup: expensesByGroup},
		&fakeSettlementRepoByGroup{},
		&fakeCommentRepoForSummary{},
		&fakeUserRepoForSummary{},
	)

	events, err := svc.GetActivity(context.Background(), 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if !events[0].Deleted {
		t.Error("expected the event to be flagged as deleted")
	}
	if events[0].DeletedByName != "Bob" {
		t.Errorf("expected deleted_by_name %q, got %q", "Bob", events[0].DeletedByName)
	}
}

func TestGetActivity_IncludesCommentsOnExpensesAndSettlements(t *testing.T) {
	groups := []domain.Group{
		{ID: 1, Name: "Asado", Currency: "ARS", Members: []domain.User{{ID: 1, Name: "Ana"}, {ID: 2, Name: "Bob"}}},
	}
	expensesByGroup := map[uint][]domain.Expense{
		1: {{ID: 9, PaidByID: 1, Description: "Carne", Amount: 5000}},
	}
	settlementsByGroup := map[uint][]domain.Settlement{
		1: {{ID: 3, FromUserID: 2, ToUserID: 1, Amount: 1000}},
	}
	expenseID := uint(9)
	settlementID := uint(3)
	commentRepo := &fakeCommentRepoForSummary{
		byExpenseID: map[uint][]domain.Comment{
			9: {{ID: 100, ExpenseID: &expenseID, UserID: 2, Body: "Gracias!", User: domain.User{ID: 2, Name: "Bob"}}},
		},
		bySettlementID: map[uint][]domain.Comment{
			3: {{ID: 101, SettlementID: &settlementID, UserID: 1, Body: "Listo", User: domain.User{ID: 1, Name: "Ana"}}},
		},
	}

	svc := NewSummaryService(
		&fakeGroupRepoForSummary{groups: groups},
		&fakeExpenseRepoByGroup{byGroup: expensesByGroup},
		&fakeSettlementRepoByGroup{byGroup: settlementsByGroup},
		commentRepo,
		&fakeUserRepoForSummary{},
	)

	events, err := svc.GetActivity(context.Background(), 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var commentEvents []domain.ActivityEvent
	for _, e := range events {
		if e.Type == "comment" {
			commentEvents = append(commentEvents, e)
		}
	}
	if len(commentEvents) != 2 {
		t.Fatalf("expected 2 comment events, got %d: %+v", len(commentEvents), commentEvents)
	}

	byParentType := map[string]domain.ActivityEvent{}
	for _, e := range commentEvents {
		byParentType[e.ParentType] = e
	}

	onExpense := byParentType["expense"]
	if onExpense.ID != 9 || onExpense.Title != "Gracias!" || onExpense.ActorName != "Bob" || onExpense.ParentTitle != "Carne" {
		t.Errorf("unexpected expense-comment event: %+v", onExpense)
	}
	onSettlement := byParentType["settlement"]
	if onSettlement.ID != 3 || onSettlement.Title != "Listo" || onSettlement.ActorName != "Ana" {
		t.Errorf("unexpected settlement-comment event: %+v", onSettlement)
	}
}

func TestGetUnreadActivityCount_ExcludesOwnActionsAndOldEvents(t *testing.T) {
	lastSeen := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	groups := []domain.Group{
		{ID: 1, Name: "Asado", Members: []domain.User{{ID: 1, Name: "Ana"}, {ID: 2, Name: "Bob"}}},
	}
	expensesByGroup := map[uint][]domain.Expense{
		1: {
			// Before lastSeen — already seen, shouldn't count.
			{ID: 1, PaidByID: 2, Description: "Old", Amount: 100, CreatedAt: lastSeen.Add(-time.Hour)},
			// After lastSeen, by someone else — should count.
			{ID: 2, PaidByID: 2, Description: "New from Bob", Amount: 200, CreatedAt: lastSeen.Add(time.Hour)},
			// After lastSeen, but caused by the user themselves — shouldn't count.
			{ID: 3, PaidByID: 1, Description: "New from me", Amount: 300, CreatedAt: lastSeen.Add(2 * time.Hour)},
		},
	}

	svc := NewSummaryService(
		&fakeGroupRepoForSummary{groups: groups},
		&fakeExpenseRepoByGroup{byGroup: expensesByGroup},
		&fakeSettlementRepoByGroup{},
		&fakeCommentRepoForSummary{},
		&fakeUserRepoForSummary{usersByID: map[uint]*domain.User{
			1: {ID: 1, ActivityLastSeenAt: lastSeen},
		}},
	)

	count, err := svc.GetUnreadActivityCount(context.Background(), 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if count != 1 {
		t.Errorf("expected 1 unread event, got %d", count)
	}
}

func TestMarkActivitySeen_UpdatesUserTimestamp(t *testing.T) {
	userRepo := &fakeUserRepoForSummary{usersByID: map[uint]*domain.User{
		1: {ID: 1},
	}}
	svc := NewSummaryService(
		&fakeGroupRepoForSummary{},
		&fakeExpenseRepoByGroup{},
		&fakeSettlementRepoByGroup{},
		&fakeCommentRepoForSummary{},
		userRepo,
	)

	before := time.Now()
	if err := svc.MarkActivitySeen(context.Background(), 1); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	after := userRepo.usersByID[1].ActivityLastSeenAt
	if after.Before(before) {
		t.Errorf("expected ActivityLastSeenAt to be updated to ~now, got %v (before test started: %v)", after, before)
	}
}
