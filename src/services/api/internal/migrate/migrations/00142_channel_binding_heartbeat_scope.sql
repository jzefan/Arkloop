-- +goose Up

ALTER TABLE channel_identity_links
    ADD COLUMN IF NOT EXISTS heartbeat_enabled INTEGER NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS heartbeat_interval_minutes INTEGER NOT NULL DEFAULT 30,
    ADD COLUMN IF NOT EXISTS heartbeat_model TEXT NOT NULL DEFAULT '';

UPDATE channel_identity_links AS cil
   SET heartbeat_enabled = COALESCE(ci.heartbeat_enabled, 0),
       heartbeat_interval_minutes = COALESCE(ci.heartbeat_interval_minutes, 30),
       heartbeat_model = COALESCE(ci.heartbeat_model, '')
  FROM channel_identities AS ci
 WHERE ci.id = cil.channel_identity_id;

ALTER TABLE scheduled_triggers
    ADD COLUMN IF NOT EXISTS channel_id UUID;

DROP INDEX IF EXISTS scheduled_triggers_channel_identity_id_idx;

INSERT INTO scheduled_triggers (
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
WITH target_persona AS (
    SELECT DISTINCT ON (account_id, key)
           id,
           account_id,
           key
      FROM personas
     WHERE deleted_at IS NULL
     ORDER BY account_id, key, created_at DESC
)
SELECT
    gen_random_uuid(),
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
  JOIN target_persona AS tp
    ON tp.account_id = st.account_id
   AND tp.key = st.persona_key
  JOIN channel_group_threads AS cgt
    ON cgt.platform_chat_id = ci.platform_subject_id
   AND cgt.persona_id = tp.id
  JOIN threads AS t
    ON t.id = cgt.thread_id
 WHERE st.channel_id IS NULL
   AND t.account_id = st.account_id
   AND t.deleted_at IS NULL;

INSERT INTO scheduled_triggers (
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
SELECT DISTINCT
    gen_random_uuid(),
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
 WHERE st.channel_id IS NULL
   AND ch.account_id = st.account_id;

DELETE FROM scheduled_triggers
 WHERE channel_id IS NULL;

ALTER TABLE scheduled_triggers
    ALTER COLUMN channel_id SET NOT NULL;

CREATE UNIQUE INDEX IF NOT EXISTS scheduled_triggers_target_idx
    ON scheduled_triggers (channel_id, channel_identity_id);

-- +goose Down

DROP INDEX IF EXISTS scheduled_triggers_target_idx;

DELETE FROM scheduled_triggers;

ALTER TABLE scheduled_triggers
    DROP COLUMN IF EXISTS channel_id;

CREATE UNIQUE INDEX IF NOT EXISTS scheduled_triggers_channel_identity_id_idx
    ON scheduled_triggers (channel_identity_id);

ALTER TABLE channel_identity_links
    DROP COLUMN IF EXISTS heartbeat_model,
    DROP COLUMN IF EXISTS heartbeat_interval_minutes,
    DROP COLUMN IF EXISTS heartbeat_enabled;
