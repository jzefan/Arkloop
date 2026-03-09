-- +goose Up

ALTER TABLE shell_sessions
    DROP COLUMN IF EXISTS latest_checkpoint_rev;

-- +goose Down

ALTER TABLE shell_sessions
    ADD COLUMN IF NOT EXISTS latest_checkpoint_rev TEXT NULL;
