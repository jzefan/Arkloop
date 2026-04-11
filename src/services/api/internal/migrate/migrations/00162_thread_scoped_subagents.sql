-- +goose Up

ALTER TABLE sub_agents
    ADD COLUMN IF NOT EXISTS owner_thread_id UUID REFERENCES threads(id) ON DELETE CASCADE,
    ADD COLUMN IF NOT EXISTS agent_thread_id UUID REFERENCES threads(id) ON DELETE CASCADE,
    ADD COLUMN IF NOT EXISTS origin_run_id UUID REFERENCES runs(id) ON DELETE CASCADE,
    ADD COLUMN IF NOT EXISTS parent_sub_agent_id UUID REFERENCES sub_agents(id) ON DELETE SET NULL;

UPDATE sub_agents sa
   SET owner_thread_id = COALESCE(sa.owner_thread_id, sa.parent_thread_id, sa.root_thread_id),
       agent_thread_id = COALESCE(
           sa.agent_thread_id,
           (SELECT r.thread_id FROM runs r WHERE r.id = sa.current_run_id),
           (SELECT r.thread_id FROM runs r WHERE r.id = sa.last_completed_run_id),
           sa.parent_thread_id,
           sa.root_thread_id
       ),
       origin_run_id = COALESCE(sa.origin_run_id, sa.parent_run_id)
 WHERE sa.owner_thread_id IS NULL
    OR sa.agent_thread_id IS NULL
    OR sa.origin_run_id IS NULL;

ALTER TABLE sub_agents
    ALTER COLUMN owner_thread_id SET NOT NULL,
    ALTER COLUMN agent_thread_id SET NOT NULL,
    ALTER COLUMN origin_run_id SET NOT NULL;

DROP INDEX IF EXISTS idx_sub_agents_parent_run_id;
DROP INDEX IF EXISTS idx_sub_agents_root_run_id;
CREATE INDEX IF NOT EXISTS idx_sub_agents_owner_thread_id ON sub_agents(owner_thread_id);
CREATE INDEX IF NOT EXISTS idx_sub_agents_parent_sub_agent_id ON sub_agents(parent_sub_agent_id) WHERE parent_sub_agent_id IS NOT NULL;

ALTER TABLE sub_agents
    DROP COLUMN IF EXISTS parent_run_id,
    DROP COLUMN IF EXISTS parent_thread_id,
    DROP COLUMN IF EXISTS root_run_id,
    DROP COLUMN IF EXISTS root_thread_id;

CREATE TABLE thread_subagent_callbacks (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    account_id UUID NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    thread_id UUID NOT NULL REFERENCES threads(id) ON DELETE CASCADE,
    sub_agent_id UUID NOT NULL REFERENCES sub_agents(id) ON DELETE CASCADE,
    source_run_id UUID NOT NULL REFERENCES runs(id) ON DELETE CASCADE,
    status TEXT NOT NULL,
    payload_json JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    consumed_at TIMESTAMPTZ NULL,
    consumed_by_run_id UUID NULL REFERENCES runs(id) ON DELETE SET NULL
);

CREATE INDEX idx_thread_subagent_callbacks_thread_pending
    ON thread_subagent_callbacks(thread_id, created_at)
    WHERE consumed_at IS NULL;

-- +goose Down

DROP INDEX IF EXISTS idx_thread_subagent_callbacks_thread_pending;
DROP TABLE IF EXISTS thread_subagent_callbacks;

ALTER TABLE sub_agents
    ADD COLUMN IF NOT EXISTS parent_run_id UUID REFERENCES runs(id) ON DELETE CASCADE,
    ADD COLUMN IF NOT EXISTS parent_thread_id UUID REFERENCES threads(id) ON DELETE CASCADE,
    ADD COLUMN IF NOT EXISTS root_run_id UUID REFERENCES runs(id) ON DELETE CASCADE,
    ADD COLUMN IF NOT EXISTS root_thread_id UUID REFERENCES threads(id) ON DELETE CASCADE;

UPDATE sub_agents
   SET parent_run_id = origin_run_id,
       parent_thread_id = owner_thread_id,
       root_run_id = origin_run_id,
       root_thread_id = owner_thread_id
 WHERE parent_run_id IS NULL
    OR parent_thread_id IS NULL
    OR root_run_id IS NULL
    OR root_thread_id IS NULL;

ALTER TABLE sub_agents
    ALTER COLUMN parent_run_id SET NOT NULL,
    ALTER COLUMN parent_thread_id SET NOT NULL,
    ALTER COLUMN root_run_id SET NOT NULL,
    ALTER COLUMN root_thread_id SET NOT NULL;

DROP INDEX IF EXISTS idx_sub_agents_owner_thread_id;
DROP INDEX IF EXISTS idx_sub_agents_parent_sub_agent_id;
CREATE INDEX IF NOT EXISTS idx_sub_agents_parent_run_id ON sub_agents(parent_run_id);
CREATE INDEX IF NOT EXISTS idx_sub_agents_root_run_id ON sub_agents(root_run_id);

ALTER TABLE sub_agents
    DROP COLUMN IF EXISTS owner_thread_id,
    DROP COLUMN IF EXISTS agent_thread_id,
    DROP COLUMN IF EXISTS origin_run_id,
    DROP COLUMN IF EXISTS parent_sub_agent_id;
