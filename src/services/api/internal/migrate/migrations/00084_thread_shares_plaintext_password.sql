-- +goose Up
ALTER TABLE thread_shares RENAME COLUMN password_hash TO password;

-- +goose Down
ALTER TABLE thread_shares RENAME COLUMN password TO password_hash;
