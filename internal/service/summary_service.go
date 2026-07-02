package service

import (
	"context"
	"sort"

	"spliteasy/internal/domain"
	"spliteasy/internal/repository"
)

type SummaryService interface {
	GetHomeSummary(ctx context.Context, userID uint) (*domain.HomeSummary, error)
	GetActivity(ctx context.Context, userID uint) ([]domain.ActivityEvent, error)
}

type summaryService struct {
	groupRepo      repository.GroupRepository
	expenseRepo    repository.ExpenseRepository
	settlementRepo repository.SettlementRepository
}

func NewSummaryService(
	groupRepo repository.GroupRepository,
	expenseRepo repository.ExpenseRepository,
	settlementRepo repository.SettlementRepository,
) SummaryService {
	return &summaryService{groupRepo, expenseRepo, settlementRepo}
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

func (s *summaryService) GetActivity(ctx context.Context, userID uint) ([]domain.ActivityEvent, error) {
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

		expenses, err := s.expenseRepo.GetByGroupID(ctx, g.ID)
		if err != nil {
			return nil, err
		}
		for _, e := range expenses {
			var yourShare int64
			for _, sp := range e.Splits {
				if sp.UserID == userID {
					yourShare = sp.Amount
				}
			}
			events = append(events, domain.ActivityEvent{
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
			})
		}

		settlements, err := s.settlementRepo.GetByGroupID(ctx, g.ID)
		if err != nil {
			return nil, err
		}
		for _, st := range settlements {
			events = append(events, domain.ActivityEvent{
				ID:         st.ID,
				Type:       "settlement",
				GroupID:    g.ID,
				GroupName:  g.Name,
				GroupEmoji: g.Emoji,
				Currency:   g.Currency,
				Title:      names[st.FromUserID] + " paid " + names[st.ToUserID],
				ActorID:    st.FromUserID,
				ActorName:  names[st.FromUserID],
				Amount:     st.Amount,
				Date:       st.CreatedAt,
			})
		}
	}

	// Newest first.
	sort.Slice(events, func(i, j int) bool {
		return events[i].Date.After(events[j].Date)
	})

	const maxEvents = 40
	if len(events) > maxEvents {
		events = events[:maxEvents]
	}

	return events, nil
}
