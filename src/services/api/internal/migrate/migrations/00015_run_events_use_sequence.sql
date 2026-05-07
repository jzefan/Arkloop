-- +goose Up
CREATE SEQUENCE run_events_seq_global;

-- 将序号对齐到现有最大值，避免与已有数据冲突
-- setval 第三个参数 false：nextval 将返回正好等于该值（即 max+1）
SELECT setval('run_events_seq_global', COALESCE(MAX(seq), 0) + 1, false)
FROM run_events;

ALTER TABLE run_events
    ALTER COLUMN seq SET DEFAULT nextval('run_events_seq_global');

-- +goose Down
ALTER TABLE run_events
    ALTER COLUMN seq DROP DEFAULT;

DROP SEQUENCE IF EXISTS run_events_seq_global;
