ALTER TABLE users ADD COLUMN push_expenses_enabled BOOLEAN NOT NULL DEFAULT true;
ALTER TABLE users ADD COLUMN push_payments_enabled BOOLEAN NOT NULL DEFAULT true;
ALTER TABLE users ADD COLUMN push_comments_enabled BOOLEAN NOT NULL DEFAULT true;
