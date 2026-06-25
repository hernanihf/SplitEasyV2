package service

import (
	"context"
	"math"
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
func userNet(userID uint, expenses []domain.Expense, settlements []domain.Settlement) float64 {
	net := 0.0
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
	return math.Round(net*100) / 100
}

func (s *summaryService) GetHomeSummary(ctx context.Context, userID uint) (*domain.HomeSummary, error) {
	groups, err := s.groupRepo.GetByUserID(ctx, userID)
	if err != nil {
		return nil, err
	}

	summary := &domain.HomeSummary{Groups: []domain.GroupSummary{}}

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
			MembersCount: len(g.Members),
			YourBalance:  net,
		})

		summary.Overall.Net += net
		if net > 0 {
			summary.Overall.Owed += net
		} else {
			summary.Overall.Owe += -net
		}
	}

	summary.Overall.Net = math.Round(summary.Overall.Net*100) / 100
	summary.Overall.Owed = math.Round(summary.Overall.Owed*100) / 100
	summary.Overall.Owe = math.Round(summary.Overall.Owe*100) / 100

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
			var yourShare float64
			for _, sp := range e.Splits {
				if sp.UserID == userID {
					yourShare = sp.Amount
				}
			}
			events = append(events, domain.ActivityEvent{
				Type:       "expense",
				GroupID:    g.ID,
				GroupName:  g.Name,
				GroupEmoji: g.Emoji,
				Title:      e.Description,
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
				Type:       "settlement",
				GroupID:    g.ID,
				GroupName:  g.Name,
				GroupEmoji: g.Emoji,
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
