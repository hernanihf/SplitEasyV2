package service

import (
	"context"
	"log/slog"
	"time"

	"spliteasy/internal/repository"
)

// ExpenseRetentionDays is how long a soft-deleted expense sticks around
// before PurgeService removes it permanently.
const ExpenseRetentionDays = 60

// purgeJobMinInterval throttles how often the purge actually runs, no matter
// how many instances are calling PurgeOldExpenses or how often — it's the
// window JobRunRepository.TryClaim uses to decide whether this call is the
// one that gets to do the work. Kept under 24h (with slack for a slow run or
// a delayed wake-up) so the purge still runs roughly once a day.
const purgeJobMinInterval = 23 * time.Hour

const purgeJobName = "purge_old_expenses"

// PurgeService permanently deletes expenses (and their receipt images) that
// have been soft-deleted for longer than ExpenseRetentionDays.
type PurgeService interface {
	// PurgeOldExpenses claims the purge job (via JobRunRepository) and, if it
	// wins the claim, deletes every expense soft-deleted more than
	// ExpenseRetentionDays ago along with its receipt image. Safe to call
	// from multiple instances/goroutines concurrently or on a schedule —
	// at most one caller across the whole fleet actually does the work
	// within purgeJobMinInterval.
	PurgeOldExpenses(ctx context.Context) error
}

type purgeService struct {
	expenseRepo    repository.ExpenseRepository
	jobRunRepo     repository.JobRunRepository
	storageService StorageService // nil when Supabase Storage isn't configured
}

func NewPurgeService(expenseRepo repository.ExpenseRepository, jobRunRepo repository.JobRunRepository, storageService StorageService) PurgeService {
	return &purgeService{expenseRepo, jobRunRepo, storageService}
}

func (s *purgeService) PurgeOldExpenses(ctx context.Context) error {
	claimed, err := s.jobRunRepo.TryClaim(ctx, purgeJobName, purgeJobMinInterval)
	if err != nil {
		return err
	}
	if !claimed {
		return nil
	}

	cutoff := time.Now().AddDate(0, 0, -ExpenseRetentionDays)

	paths, err := s.expenseRepo.GetOldSoftDeletedReceiptImagePaths(ctx, cutoff)
	if err != nil {
		return err
	}

	purged, err := s.expenseRepo.PurgeOldSoftDeleted(ctx, cutoff)
	if err != nil {
		return err
	}
	slog.Info("purged old soft-deleted expenses", "count", purged)

	if s.storageService != nil {
		for _, path := range paths {
			if err := s.storageService.Delete(ctx, path); err != nil {
				slog.Error("failed to delete receipt image for a purged expense", "error", err, "path", path)
			}
		}
	}

	return nil
}
