ALTER TABLE expenses ADD COLUMN deleted_at TIMESTAMPTZ;
CREATE INDEX idx_expenses_deleted_at ON expenses (deleted_at);

ALTER TABLE settlements ADD COLUMN deleted_at TIMESTAMPTZ;
CREATE INDEX idx_settlements_deleted_at ON settlements (deleted_at);
