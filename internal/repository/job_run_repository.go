package repository

import (
	"context"
	"time"

	"gorm.io/gorm"
)

// JobRunRepository coordinates periodic background jobs across however many
// instances of the API happen to be running at once — TryClaim ensures only
// one instance actually does the work within a given time window, using a
// plain SQL compare-and-swap (no Postgres extensions, no session-held locks),
// so it behaves the same regardless of hosting provider.
type JobRunRepository interface {
	// TryClaim atomically claims jobName if it hasn't been claimed within
	// minInterval, returning true if this call won the claim (the caller
	// should proceed) or false if another instance already claimed it
	// recently (the caller should skip this cycle).
	TryClaim(ctx context.Context, jobName string, minInterval time.Duration) (bool, error)
}

type jobRunRepository struct {
	db *gorm.DB
}

func NewJobRunRepository(db *gorm.DB) JobRunRepository {
	return &jobRunRepository{db}
}

// TryClaim is a single atomic statement: insert the row on the job's very
// first run, or update it only if last_run_at is older than cutoff.
// Postgres serializes concurrent conflicts on the same key, so when two
// instances race, exactly one of them sees RowsAffected > 0.
func (r *jobRunRepository) TryClaim(ctx context.Context, jobName string, minInterval time.Duration) (bool, error) {
	cutoff := time.Now().Add(-minInterval)
	result := r.db.WithContext(ctx).Exec(`
		INSERT INTO job_runs (job_name, last_run_at) VALUES (?, now())
		ON CONFLICT (job_name) DO UPDATE
		SET last_run_at = now()
		WHERE job_runs.last_run_at < ?
	`, jobName, cutoff)
	if result.Error != nil {
		return false, result.Error
	}
	return result.RowsAffected > 0, nil
}
