-- Baseline migration: reproduces the schema exactly as it exists in
-- production today (created by GORM's AutoMigrate before migrations were
-- adopted). On a fresh database this creates everything from scratch; on the
-- existing production database, version 1 is recorded as already-applied
-- (see the migration cutover notes) so this file never actually runs there.

CREATE TABLE users (
    id BIGSERIAL PRIMARY KEY,
    name TEXT NOT NULL,
    email TEXT NOT NULL UNIQUE,
    avatar_url TEXT,
    created_at TIMESTAMPTZ,
    updated_at TIMESTAMPTZ
);

CREATE TABLE groups (
    id BIGSERIAL PRIMARY KEY,
    name TEXT NOT NULL,
    emoji TEXT,
    created_by BIGINT,
    invite_token TEXT,
    created_at TIMESTAMPTZ,
    updated_at TIMESTAMPTZ
);
CREATE UNIQUE INDEX idx_groups_invite_token ON groups (invite_token);

CREATE TABLE group_users (
    group_id BIGINT NOT NULL REFERENCES groups (id),
    user_id BIGINT NOT NULL REFERENCES users (id),
    PRIMARY KEY (group_id, user_id)
);

CREATE TABLE expenses (
    id BIGSERIAL PRIMARY KEY,
    group_id BIGINT NOT NULL REFERENCES groups (id),
    paid_by_id BIGINT NOT NULL REFERENCES users (id),
    description TEXT NOT NULL,
    amount BIGINT NOT NULL,
    created_at TIMESTAMPTZ,
    updated_at TIMESTAMPTZ
);

CREATE TABLE expense_splits (
    id BIGSERIAL PRIMARY KEY,
    expense_id BIGINT NOT NULL REFERENCES expenses (id),
    user_id BIGINT NOT NULL REFERENCES users (id),
    amount BIGINT NOT NULL,
    created_at TIMESTAMPTZ,
    updated_at TIMESTAMPTZ
);

CREATE TABLE expense_items (
    id BIGSERIAL PRIMARY KEY,
    expense_id BIGINT NOT NULL REFERENCES expenses (id),
    description TEXT NOT NULL,
    amount BIGINT NOT NULL
);

CREATE TABLE expense_item_users (
    expense_item_id BIGINT NOT NULL REFERENCES expense_items (id),
    user_id BIGINT NOT NULL REFERENCES users (id),
    PRIMARY KEY (expense_item_id, user_id)
);

CREATE TABLE settlements (
    id BIGSERIAL PRIMARY KEY,
    group_id BIGINT NOT NULL,
    from_user_id BIGINT NOT NULL REFERENCES users (id),
    to_user_id BIGINT NOT NULL REFERENCES users (id),
    amount BIGINT NOT NULL,
    created_at TIMESTAMPTZ
);
