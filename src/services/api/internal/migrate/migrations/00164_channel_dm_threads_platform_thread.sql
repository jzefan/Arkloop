-- +goose Up

ALTER TABLE channel_dm_threads
    ADD COLUMN IF NOT EXISTS platform_thread_id TEXT NOT NULL DEFAULT '';

ALTER TABLE channel_dm_threads
    DROP CONSTRAINT IF EXISTS uq_channel_dm_threads_binding;

ALTER TABLE channel_dm_threads
    ADD CONSTRAINT uq_channel_dm_threads_binding
        UNIQUE (channel_id, channel_identity_id, persona_id, platform_thread_id);

-- +goose Down

ALTER TABLE channel_dm_threads
    DROP CONSTRAINT IF EXISTS uq_channel_dm_threads_binding;

ALTER TABLE channel_dm_threads
    ADD CONSTRAINT uq_channel_dm_threads_binding
        UNIQUE (channel_id, channel_identity_id, persona_id);

ALTER TABLE channel_dm_threads
    DROP COLUMN IF EXISTS platform_thread_id;
