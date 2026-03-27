-- +goose Up

CREATE TABLE channel_identity_links_v2 (
    id                  TEXT PRIMARY KEY DEFAULT (lower(hex(randomblob(4))) || '-' || lower(hex(randomblob(2))) || '-4' || substr(lower(hex(randomblob(2))),2) || '-' || substr('89ab',abs(random()) % 4 + 1, 1) || substr(lower(hex(randomblob(2))),2) || '-' || lower(hex(randomblob(6)))),
    channel_id          TEXT NOT NULL REFERENCES channels(id) ON DELETE CASCADE,
    channel_identity_id TEXT NOT NULL REFERENCES channel_identities(id) ON DELETE CASCADE,
    heartbeat_enabled   INTEGER NOT NULL DEFAULT 0,
    heartbeat_interval_minutes INTEGER NOT NULL DEFAULT 30,
    heartbeat_model     TEXT NOT NULL DEFAULT '',
    created_at          TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at          TEXT NOT NULL DEFAULT (datetime('now')),
    UNIQUE (channel_id, channel_identity_id)
);

INSERT INTO channel_identity_links_v2 (
    id,
    channel_id,
    channel_identity_id,
    heartbeat_enabled,
    heartbeat_interval_minutes,
    heartbeat_model,
    created_at,
    updated_at
)
SELECT
    cil.id,
    cil.channel_id,
    cil.channel_identity_id,
    COALESCE(ci.heartbeat_enabled, 0),
    COALESCE(ci.heartbeat_interval_minutes, 30),
    COALESCE(ci.heartbeat_model, ''),
    cil.created_at,
    cil.updated_at
  FROM channel_identity_links AS cil
  LEFT JOIN channel_identities AS ci
    ON ci.id = cil.channel_identity_id;

DROP TABLE channel_identity_links;

ALTER TABLE channel_identity_links_v2 RENAME TO channel_identity_links;

CREATE INDEX IF NOT EXISTS idx_channel_identity_links_channel_id
    ON channel_identity_links(channel_id);

CREATE INDEX IF NOT EXISTS idx_channel_identity_links_identity_id
    ON channel_identity_links(channel_identity_id);

CREATE TABLE scheduled_triggers_v2 (
    id                    TEXT PRIMARY KEY,
    channel_id            TEXT NOT NULL,
    channel_identity_id   TEXT NOT NULL,
    persona_key           TEXT NOT NULL,
    account_id            TEXT NOT NULL,
    model                 TEXT NOT NULL DEFAULT '',
    interval_min          INTEGER NOT NULL DEFAULT 30,
    next_fire_at          TEXT NOT NULL,
    created_at            TEXT NOT NULL,
    updated_at            TEXT NOT NULL,
    UNIQUE (channel_id, channel_identity_id)
);

INSERT OR IGNORE INTO scheduled_triggers_v2 (
    id,
    channel_id,
    channel_identity_id,
    persona_key,
    account_id,
    model,
    interval_min,
    next_fire_at,
    created_at,
    updated_at
)
SELECT
    lower(hex(randomblob(4))) || '-' || lower(hex(randomblob(2))) || '-4' || substr(lower(hex(randomblob(2))),2) || '-' || substr('89ab',abs(random()) % 4 + 1, 1) || substr(lower(hex(randomblob(2))),2) || '-' || lower(hex(randomblob(6))),
    cgt.channel_id,
    st.channel_identity_id,
    st.persona_key,
    st.account_id,
    st.model,
    st.interval_min,
    st.next_fire_at,
    st.created_at,
    st.updated_at
  FROM scheduled_triggers AS st
  JOIN channel_identities AS ci
    ON ci.id = st.channel_identity_id
  JOIN channel_group_threads AS cgt
    ON cgt.platform_chat_id = ci.platform_subject_id
  JOIN threads AS t
    ON t.id = cgt.thread_id
 WHERE t.account_id = st.account_id
   AND t.deleted_at IS NULL;

INSERT OR IGNORE INTO scheduled_triggers_v2 (
    id,
    channel_id,
    channel_identity_id,
    persona_key,
    account_id,
    model,
    interval_min,
    next_fire_at,
    created_at,
    updated_at
)
SELECT
    lower(hex(randomblob(4))) || '-' || lower(hex(randomblob(2))) || '-4' || substr(lower(hex(randomblob(2))),2) || '-' || substr('89ab',abs(random()) % 4 + 1, 1) || substr(lower(hex(randomblob(2))),2) || '-' || lower(hex(randomblob(6))),
    cil.channel_id,
    st.channel_identity_id,
    st.persona_key,
    st.account_id,
    st.model,
    st.interval_min,
    st.next_fire_at,
    st.created_at,
    st.updated_at
  FROM scheduled_triggers AS st
  JOIN channel_identity_links AS cil
    ON cil.channel_identity_id = st.channel_identity_id
  JOIN channels AS ch
    ON ch.id = cil.channel_id
 WHERE ch.account_id = st.account_id;

DROP TABLE scheduled_triggers;

ALTER TABLE scheduled_triggers_v2 RENAME TO scheduled_triggers;

CREATE INDEX IF NOT EXISTS scheduled_triggers_target_idx
    ON scheduled_triggers (channel_id, channel_identity_id);

CREATE INDEX IF NOT EXISTS scheduled_triggers_next_fire_at_idx
    ON scheduled_triggers (next_fire_at);

-- +goose Down

CREATE TABLE scheduled_triggers_v1 (
    id                    TEXT PRIMARY KEY,
    channel_identity_id   TEXT NOT NULL UNIQUE,
    persona_key           TEXT NOT NULL,
    account_id            TEXT NOT NULL,
    model                 TEXT NOT NULL DEFAULT '',
    interval_min          INTEGER NOT NULL DEFAULT 30,
    next_fire_at          TEXT NOT NULL,
    created_at            TEXT NOT NULL,
    updated_at            TEXT NOT NULL
);

INSERT INTO scheduled_triggers_v1 (
    id,
    channel_identity_id,
    persona_key,
    account_id,
    model,
    interval_min,
    next_fire_at,
    created_at,
    updated_at
)
SELECT
    id,
    channel_identity_id,
    persona_key,
    account_id,
    model,
    interval_min,
    next_fire_at,
    created_at,
    updated_at
  FROM (
    SELECT
        st.*,
        ROW_NUMBER() OVER (
            PARTITION BY st.channel_identity_id
            ORDER BY st.updated_at DESC, st.created_at DESC, st.id DESC
        ) AS rn
      FROM scheduled_triggers AS st
  )
 WHERE rn = 1;

DROP TABLE scheduled_triggers;

ALTER TABLE scheduled_triggers_v1 RENAME TO scheduled_triggers;

CREATE INDEX IF NOT EXISTS scheduled_triggers_next_fire_at_idx
    ON scheduled_triggers (next_fire_at);

CREATE TABLE channel_identity_links_v1 (
    id                  TEXT PRIMARY KEY DEFAULT (lower(hex(randomblob(4))) || '-' || lower(hex(randomblob(2))) || '-4' || substr(lower(hex(randomblob(2))),2) || '-' || substr('89ab',abs(random()) % 4 + 1, 1) || substr(lower(hex(randomblob(2))),2) || '-' || lower(hex(randomblob(6)))),
    channel_id          TEXT NOT NULL REFERENCES channels(id) ON DELETE CASCADE,
    channel_identity_id TEXT NOT NULL REFERENCES channel_identities(id) ON DELETE CASCADE,
    created_at          TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at          TEXT NOT NULL DEFAULT (datetime('now')),
    UNIQUE (channel_id, channel_identity_id)
);

INSERT INTO channel_identity_links_v1 (
    id,
    channel_id,
    channel_identity_id,
    created_at,
    updated_at
)
SELECT
    id,
    channel_id,
    channel_identity_id,
    created_at,
    updated_at
  FROM channel_identity_links;

DROP TABLE channel_identity_links;

ALTER TABLE channel_identity_links_v1 RENAME TO channel_identity_links;

CREATE INDEX IF NOT EXISTS idx_channel_identity_links_channel_id
    ON channel_identity_links(channel_id);

CREATE INDEX IF NOT EXISTS idx_channel_identity_links_identity_id
    ON channel_identity_links(channel_identity_id);
