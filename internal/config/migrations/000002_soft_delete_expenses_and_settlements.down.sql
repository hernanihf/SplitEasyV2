DROP INDEX IF EXISTS idx_settlements_deleted_at;
ALTER TABLE settlements DROP COLUMN IF EXISTS deleted_at;

DROP INDEX IF EXISTS idx_expenses_deleted_at;
ALTER TABLE expenses DROP COLUMN IF EXISTS deleted_at;
