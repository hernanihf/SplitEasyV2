CREATE TABLE comments (
    id BIGSERIAL PRIMARY KEY,
    expense_id BIGINT REFERENCES expenses (id),
    settlement_id BIGINT REFERENCES settlements (id),
    user_id BIGINT NOT NULL REFERENCES users (id),
    body TEXT NOT NULL,
    created_at TIMESTAMPTZ,
    updated_at TIMESTAMPTZ,
    deleted_at TIMESTAMPTZ,
    CONSTRAINT comments_exactly_one_parent CHECK (
        (expense_id IS NOT NULL AND settlement_id IS NULL) OR
        (expense_id IS NULL AND settlement_id IS NOT NULL)
    )
);
CREATE INDEX idx_comments_expense_id ON comments (expense_id);
CREATE INDEX idx_comments_settlement_id ON comments (settlement_id);
CREATE INDEX idx_comments_deleted_at ON comments (deleted_at);
