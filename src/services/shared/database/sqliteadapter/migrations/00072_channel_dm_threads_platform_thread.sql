-- +goose Up

PRAGMA foreign_keys = OFF;

ALTER TABLE channel_dm_threads RENAME TO channel_dm_threads_old_00072;

CREATE TABLE channel_dm_threads (
    id                  TEXT PRIMARY KEY DEFAULT (lower(hex(randomblob(4))) || '-' || lower(hex(randomblob(2))) || '-4' || substr(lower(hex(randomblob(2))),2) || '-' || substr('89ab',abs(random()) % 4 + 1, 1) || substr(lower(hex(randomblob(2))),2) || '-' || lower(hex(randomblob(6)))),
    channel_id          TEXT NOT NULL REFERENCES channels(id) ON DELETE CASCADE,
    channel_identity_id TEXT NOT NULL REFERENCES channel_identities(id) ON DELETE CASCADE,
    persona_id          TEXT NOT NULL REFERENCES personas(id) ON DELETE CASCADE,
    platform_thread_id  TEXT NOT NULL DEFAULT '',
    thread_id           TEXT NOT NULL REFERENCES threads(id) ON DELETE CASCADE,
    created_at          TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at          TEXT NOT NULL DEFAULT (datetime('now')),
    UNIQUE (channel_id, channel_identity_id, persona_id, platform_thread_id),
    UNIQUE (thread_id)
);

INSERT INTO channel_dm_threads (
    id,
    channel_id,
    channel_identity_id,
    persona_id,
    platform_thread_id,
    thread_id,
    created_at,
    updated_at
)
SELECT
    id,
    channel_id,
    channel_identity_id,
    persona_id,
    '',
    thread_id,
    created_at,
    updated_at
FROM channel_dm_threads_old_00072;

DROP TABLE channel_dm_threads_old_00072;

CREATE INDEX idx_channel_dm_threads_channel_identity ON channel_dm_threads(channel_identity_id);
CREATE INDEX idx_channel_dm_threads_channel_id ON channel_dm_threads(channel_id);

PRAGMA foreign_keys = ON;

-- +goose Down

PRAGMA foreign_keys = OFF;

ALTER TABLE channel_dm_threads RENAME TO channel_dm_threads_old_00072;

CREATE TABLE channel_dm_threads (
    id                  TEXT PRIMARY KEY DEFAULT (lower(hex(randomblob(4))) || '-' || lower(hex(randomblob(2))) || '-4' || substr(lower(hex(randomblob(2))),2) || '-' || substr('89ab',abs(random()) % 4 + 1, 1) || substr(lower(hex(randomblob(2))),2) || '-' || lower(hex(randomblob(6)))),
    channel_id          TEXT NOT NULL REFERENCES channels(id) ON DELETE CASCADE,
    channel_identity_id TEXT NOT NULL REFERENCES channel_identities(id) ON DELETE CASCADE,
    persona_id          TEXT NOT NULL REFERENCES personas(id) ON DELETE CASCADE,
    thread_id           TEXT NOT NULL REFERENCES threads(id) ON DELETE CASCADE,
    created_at          TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at          TEXT NOT NULL DEFAULT (datetime('now')),
    UNIQUE (channel_id, channel_identity_id, persona_id),
    UNIQUE (thread_id)
);

INSERT INTO channel_dm_threads (
    id,
    channel_id,
    channel_identity_id,
    persona_id,
    thread_id,
    created_at,
    updated_at
)
SELECT
    id,
    channel_id,
    channel_identity_id,
    persona_id,
    thread_id,
    created_at,
    updated_at
FROM channel_dm_threads_old_00072;

DROP TABLE channel_dm_threads_old_00072;

CREATE INDEX idx_channel_dm_threads_channel_identity ON channel_dm_threads(channel_identity_id);
CREATE INDEX idx_channel_dm_threads_channel_id ON channel_dm_threads(channel_id);

PRAGMA foreign_keys = ON;
