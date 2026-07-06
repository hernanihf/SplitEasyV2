ALTER TABLE expenses ADD COLUMN deleted_by_id BIGINT REFERENCES users (id);
