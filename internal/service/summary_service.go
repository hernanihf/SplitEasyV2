package service

import (
	"context"
	"sort"
	"time"

	"spliteasy/internal/domain"
	"spliteasy/internal/repository"
)

type SummaryService interface {
	GetHomeSummary(ctx context.Context, userID uint) (*domain.HomeSummary, error)
	GetActivity(ctx context.Context, userID uint) ([]domain.ActivityEvent, error)
	// GetUnreadActivityCount counts activity events strictly after the
	// user's ActivityLastSeenAt, excluding events the user themselves
	// caused (you don't need a badge for your own actions). Unlike
	// GetActivity, this is not capped at maxEvents.
	GetUnreadActivityCount(ctx context.Context, userID uint) (int, error)
	// MarkActivitySeen records "now" as the user's activity-last-seen time,
	// clearing the unread badge.
	MarkActivitySeen(ctx context.Context, userID uint) error
}

type summaryService struct {
	groupRepo      repository.GroupRepository
	expenseRepo    repository.ExpenseRepository
	settlementRepo repository.SettlementRepository
	commentRepo    repository.CommentRepository
	userRepo       repository.UserRepository
}

func NewSummaryService(
	groupRepo repository.GroupRepository,
	expenseRepo repository.ExpenseRepository,
	settlementRepo repository.SettlementRepository,
	commentRepo repository.CommentRepository,
	userRepo repository.UserRepository,
) SummaryService {
	return &summaryService{groupRepo, expenseRepo, settlementRepo, commentRepo, userRepo}
}

// userNet returns the user's net balance for a group: positive means the group
// owes them money, negative means they owe the group.
func userNet(userID uint, expenses []domain.Expense, settlements []domain.Settlement) int64 {
	var net int64
	for _, e := range expenses {
		if e.PaidByID == userID {
			net += e.Amount
		}
		for _, s := range e.Splits {
			if s.UserID == userID {
				net -= s.Amount
			}
		}
	}
	for _, st := range settlements {
		if st.FromUserID == userID {
			net += st.Amount
		}
		if st.ToUserID == userID {
			net -= st.Amount
		}
	}
	return net
}

func (s *summaryService) GetHomeSummary(ctx context.Context, userID uint) (*domain.HomeSummary, error) {
	groups, err := s.groupRepo.GetByUserID(ctx, userID)
	if err != nil {
		return nil, err
	}

	summary := &domain.HomeSummary{Groups: []domain.GroupSummary{}}

	// Groups in different currencies can't be summed into one number without
	// a conversion rate, so totals are kept separate per currency.
	byCurrency := map[string]*domain.OverallBalance{}

	for _, g := range groups {
		expenses, err := s.expenseRepo.GetByGroupID(ctx, g.ID)
		if err != nil {
			return nil, err
		}
		settlements, err := s.settlementRepo.GetByGroupID(ctx, g.ID)
		if err != nil {
			return nil, err
		}

		net := userNet(userID, expenses, settlements)

		summary.Groups = append(summary.Groups, domain.GroupSummary{
			ID:           g.ID,
			Name:         g.Name,
			Emoji:        g.Emoji,
			Currency:     g.Currency,
			MembersCount: len(g.Members),
			YourBalance:  net,
			CreatedBy:    g.CreatedBy,
		})

		overall, ok := byCurrency[g.Currency]
		if !ok {
			overall = &domain.OverallBalance{Currency: g.Currency}
			byCurrency[g.Currency] = overall
		}
		overall.Net += net
		if net > 0 {
			overall.Owed += net
		} else {
			overall.Owe += -net
		}
	}

	currencies := make([]string, 0, len(byCurrency))
	for currency := range byCurrency {
		currencies = append(currencies, currency)
	}
	sort.Strings(currencies)
	for _, currency := range currencies {
		summary.OverallByCurrency = append(summary.OverallByCurrency, *byCurrency[currency])
	}

	return summary, nil
}

// buildActivityEvents gathers every expense, settlement, and comment event
// across the user's groups, newest first, uncapped, with IsUnread already
// set on each — shared by GetActivity (which trims to maxEvents, letting the
// feed highlight unread rows) and GetUnreadActivityCount (which needs the
// true total, not just the most recent page of it).
func (s *summaryService) buildActivityEvents(ctx context.Context, userID uint) ([]domain.ActivityEvent, error) {
	user, err := s.userRepo.GetByID(ctx, userID)
	if err != nil {
		return nil, err
	}

	groups, err := s.groupRepo.GetByUserID(ctx, userID)
	if err != nil {
		return nil, err
	}

	events := []domain.ActivityEvent{}

	for _, g := range groups {
		names := map[uint]string{}
		for _, m := range g.Members {
			names[m.ID] = m.Name
		}

		// Includes soft-deleted expenses (unlike GetHomeSummary's balance
		// calculation above) — the feed shows them struck through instead of
		// silently dropping them.
		expenses, err := s.expenseRepo.GetByGroupIDIncludingDeleted(ctx, g.ID)
		if err != nil {
			return nil, err
		}
		expenseTitles := make(map[uint]string, len(expenses))
		expenseIDs := make([]uint, 0, len(expenses))
		for _, e := range expenses {
			expenseTitles[e.ID] = e.Description
			expenseIDs = append(expenseIDs, e.ID)

			var yourShare int64
			for _, sp := range e.Splits {
				if sp.UserID == userID {
					yourShare = sp.Amount
				}
			}
			event := domain.ActivityEvent{
				ID:         e.ID,
				Type:       "expense",
				GroupID:    g.ID,
				GroupName:  g.Name,
				GroupEmoji: g.Emoji,
				Currency:   g.Currency,
				Title:      e.Description,
				Category:   e.Category,
				ActorID:    e.PaidByID,
				ActorName:  names[e.PaidByID],
				Amount:     e.Amount,
				YourShare:  yourShare,
				Date:       e.CreatedAt,
				Deleted:    e.DeletedAt.Valid,
			}
			if e.DeletedAt.Valid && e.DeletedBy != nil {
				event.DeletedByName = e.DeletedBy.Name
			}
			events = append(events, event)
		}

		settlements, err := s.settlementRepo.GetByGroupID(ctx, g.ID)
		if err != nil {
			return nil, err
		}
		settlementTitles := make(map[uint]string, len(settlements))
		settlementIDs := make([]uint, 0, len(settlements))
		for _, st := range settlements {
			title := names[st.FromUserID] + " paid " + names[st.ToUserID]
			settlementTitles[st.ID] = title
			settlementIDs = append(settlementIDs, st.ID)

			events = append(events, domain.ActivityEvent{
				ID:         st.ID,
				Type:       "settlement",
				GroupID:    g.ID,
				GroupName:  g.Name,
				GroupEmoji: g.Emoji,
				Currency:   g.Currency,
				Title:      title,
				ActorID:    st.FromUserID,
				ActorName:  names[st.FromUserID],
				Amount:     st.Amount,
				Date:       st.CreatedAt,
			})
		}

		expenseComments, err := s.commentRepo.GetByExpenseIDs(ctx, expenseIDs)
		if err != nil {
			return nil, err
		}
		for _, c := range expenseComments {
			events = append(events, domain.ActivityEvent{
				ID:          *c.ExpenseID,
				Type:        "comment",
				GroupID:     g.ID,
				GroupName:   g.Name,
				GroupEmoji:  g.Emoji,
				Currency:    g.Currency,
				Title:       c.Body,
				ActorID:     c.UserID,
				ActorName:   c.User.Name,
				Date:        c.CreatedAt,
				ParentType:  "expense",
				ParentTitle: expenseTitles[*c.ExpenseID],
			})
		}

		settlementComments, err := s.commentRepo.GetBySettlementIDs(ctx, settlementIDs)
		if err != nil {
			return nil, err
		}
		for _, c := range settlementComments {
			events = append(events, domain.ActivityEvent{
				ID:          *c.SettlementID,
				Type:        "comment",
				GroupID:     g.ID,
				GroupName:   g.Name,
				GroupEmoji:  g.Emoji,
				Currency:    g.Currency,
				Title:       c.Body,
				ActorID:     c.UserID,
				ActorName:   c.User.Name,
				Date:        c.CreatedAt,
				ParentType:  "settlement",
				ParentTitle: settlementTitles[*c.SettlementID],
			})
		}
	}

	// Newest first.
	sort.Slice(events, func(i, j int) bool {
		return events[i].Date.After(events[j].Date)
	})

	// An event is unread if it happened after the user last viewed the feed
	// and they didn't cause it themselves (no badge/highlight for your own
	// actions).
	for i := range events {
		events[i].IsUnread = events[i].Date.After(user.ActivityLastSeenAt) && events[i].ActorID != userID
	}

	return events, nil
}

func (s *summaryService) GetActivity(ctx context.Context, userID uint) ([]domain.ActivityEvent, error) {
	events, err := s.buildActivityEvents(ctx, userID)
	if err != nil {
		return nil, err
	}

	const maxEvents = 40
	if len(events) > maxEvents {
		events = events[:maxEvents]
	}

	return events, nil
}

func (s *summaryService) GetUnreadActivityCount(ctx context.Context, userID uint) (int, error) {
	events, err := s.buildActivityEvents(ctx, userID)
	if err != nil {
		return 0, err
	}

	count := 0
	for _, e := range events {
		if e.IsUnread {
			count++
		}
	}
	return count, nil
}

func (s *summaryService) MarkActivitySeen(ctx context.Context, userID uint) error {
	return s.userRepo.UpdateActivityLastSeenAt(ctx, userID, time.Now())
}
