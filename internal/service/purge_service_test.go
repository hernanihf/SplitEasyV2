package service

import (
	"context"
	"errors"
	"testing"
	"time"
)

// fakeJobRunRepo lets purge_service tests control/observe TryClaim without a
// real Postgres claim table.
type fakeJobRunRepo struct {
	claimed        bool
	claimErr       error
	claimedJobName string
}

func (f *fakeJobRunRepo) TryClaim(_ context.Context, jobName string, _ time.Duration) (bool, error) {
	f.claimedJobName = jobName
	return f.claimed, f.claimErr
}

func TestPurgeOldExpenses_SkipsWhenClaimNotWon(t *testing.T) {
	expenseRepo := &fakeExpenseRepo{purgedCount: 5}
	jobRunRepo := &fakeJobRunRepo{claimed: false}
	storage := &fakeStorageService{}

	svc := NewPurgeService(expenseRepo, jobRunRepo, storage)
	if err := svc.PurgeOldExpenses(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !expenseRepo.purgeCalledWith.IsZero() {
		t.Error("expected PurgeOldSoftDeleted not to be called when the claim isn't won")
	}
	if len(storage.deletedPaths) != 0 {
		t.Error("expected no storage deletes when the claim isn't won")
	}
}

func TestPurgeOldExpenses_ClaimWonPurgesAndCleansUpImages(t *testing.T) {
	expenseRepo := &fakeExpenseRepo{
		purgedCount:          3,
		oldReceiptImagePaths: []string{"receipts/a.jpg", "receipts/b.jpg"},
	}
	jobRunRepo := &fakeJobRunRepo{claimed: true}
	storage := &fakeStorageService{}

	svc := NewPurgeService(expenseRepo, jobRunRepo, storage)
	if err := svc.PurgeOldExpenses(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if expenseRepo.purgeCalledWith.IsZero() {
		t.Error("expected PurgeOldSoftDeleted to be called when the claim is won")
	}
	if len(storage.deletedPaths) != 2 {
		t.Fatalf("expected 2 images deleted, got %d", len(storage.deletedPaths))
	}
	if storage.deletedPaths[0] != "receipts/a.jpg" || storage.deletedPaths[1] != "receipts/b.jpg" {
		t.Errorf("unexpected deleted paths: %v", storage.deletedPaths)
	}
	if jobRunRepo.claimedJobName != purgeJobName {
		t.Errorf("expected claim for job %q, got %q", purgeJobName, jobRunRepo.claimedJobName)
	}
}

func TestPurgeOldExpenses_NilStorageDoesNotPanic(t *testing.T) {
	expenseRepo := &fakeExpenseRepo{
		purgedCount:          1,
		oldReceiptImagePaths: []string{"receipts/a.jpg"},
	}
	jobRunRepo := &fakeJobRunRepo{claimed: true}

	svc := NewPurgeService(expenseRepo, jobRunRepo, nil)
	if err := svc.PurgeOldExpenses(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestPurgeOldExpenses_StorageDeleteFailureIsNonFatal(t *testing.T) {
	expenseRepo := &fakeExpenseRepo{
		purgedCount:          1,
		oldReceiptImagePaths: []string{"receipts/a.jpg"},
	}
	jobRunRepo := &fakeJobRunRepo{claimed: true}
	storage := &fakeStorageService{deleteErr: errors.New("network error")}

	svc := NewPurgeService(expenseRepo, jobRunRepo, storage)
	if err := svc.PurgeOldExpenses(context.Background()); err != nil {
		t.Fatalf("expected purge to succeed despite storage delete failure, got: %v", err)
	}
}

func TestPurgeOldExpenses_ClaimErrorPropagates(t *testing.T) {
	expenseRepo := &fakeExpenseRepo{}
	jobRunRepo := &fakeJobRunRepo{claimErr: errors.New("db error")}
	storage := &fakeStorageService{}

	svc := NewPurgeService(expenseRepo, jobRunRepo, storage)
	if err := svc.PurgeOldExpenses(context.Background()); err == nil {
		t.Fatal("expected error to propagate from TryClaim")
	}
}

func TestPurgeOldExpenses_PurgeErrorPropagates(t *testing.T) {
	expenseRepo := &fakeExpenseRepo{purgeErr: errors.New("db error")}
	jobRunRepo := &fakeJobRunRepo{claimed: true}
	storage := &fakeStorageService{}

	svc := NewPurgeService(expenseRepo, jobRunRepo, storage)
	if err := svc.PurgeOldExpenses(context.Background()); err == nil {
		t.Fatal("expected error to propagate from PurgeOldSoftDeleted")
	}
}
