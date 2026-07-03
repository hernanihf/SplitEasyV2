CREATE TABLE job_runs (
    job_name TEXT PRIMARY KEY,
    last_run_at TIMESTAMPTZ NOT NULL DEFAULT '-infinity'
);
