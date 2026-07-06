-- group_id intentionally has no foreign key: a group can be permanently
-- deleted (DELETE /groups/{id}), and the audit trail — including the record
-- of that very deletion — must survive the group it points to being gone.
CREATE TABLE audit_logs (
    id BIGSERIAL PRIMARY KEY,
    group_id BIGINT NOT NULL,
    actor_id BIGINT NOT NULL REFERENCES users (id),
    action TEXT NOT NULL,
    entity_type TEXT NOT NULL,
    entity_id BIGINT NOT NULL,
    detail TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_audit_logs_group_id_created_at ON audit_logs (group_id, created_at DESC);
