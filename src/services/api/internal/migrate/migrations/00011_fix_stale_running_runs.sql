-- +goose Up
-- 修复历史遗留的 stale runs：status 仍为 running 但已有 terminal event
UPDATE runs r
SET status = CASE e.type
    WHEN 'run.completed' THEN 'completed'
    WHEN 'run.failed'    THEN 'failed'
    WHEN 'run.cancelled' THEN 'cancelled'
END
FROM (
    SELECT DISTINCT ON (run_id) run_id, type
    FROM run_events
    WHERE type IN ('run.completed', 'run.failed', 'run.cancelled')
    ORDER BY run_id, seq DESC
) e
WHERE r.id = e.run_id
  AND r.status = 'running';

-- +goose Down
-- 原始 status 已丢失，不可回滚
