CREATE TABLE push_subscriptions (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES users (id),
    endpoint TEXT NOT NULL,
    p256dh TEXT NOT NULL,
    auth TEXT NOT NULL,
    created_at TIMESTAMPTZ,
    CONSTRAINT push_subscriptions_user_endpoint_unique UNIQUE (user_id, endpoint)
);
CREATE INDEX idx_push_subscriptions_user_id ON push_subscriptions (user_id);
