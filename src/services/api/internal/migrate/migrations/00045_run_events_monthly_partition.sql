-- +goose Up

-- 1. 创建分区父表，结构与现有 run_events 一致
--    PK 和 UNIQUE 必须包含分区键 ts
CREATE TABLE run_events_partitioned (
    event_id    UUID         DEFAULT gen_random_uuid(),
    run_id      UUID         NOT NULL REFERENCES runs(id) ON DELETE CASCADE,
    seq         BIGINT       NOT NULL DEFAULT nextval('run_events_seq_global'),
    ts          TIMESTAMPTZ  NOT NULL DEFAULT now(),
    type        TEXT         NOT NULL,
    data_json   JSONB        NOT NULL DEFAULT '{}'::jsonb,
    tool_name   TEXT,
    error_class TEXT,
    CONSTRAINT pk_run_events_part PRIMARY KEY (event_id, ts),
    CONSTRAINT uq_run_events_part_run_id_seq UNIQUE (run_id, seq, ts)
) PARTITION BY RANGE (ts);

-- 2. DEFAULT 分区兜底历史数据和异常时间戳
CREATE TABLE run_events_pdefault PARTITION OF run_events_partitioned DEFAULT;

-- 3. 动态创建当前月和下月分区
-- +goose StatementBegin
DO $body$
DECLARE
    curr_start DATE := date_trunc('month', now())::DATE;
    next_start DATE := (date_trunc('month', now()) + INTERVAL '1 month')::DATE;
    after_start DATE := (date_trunc('month', now()) + INTERVAL '2 months')::DATE;
BEGIN
    EXECUTE format(
        'CREATE TABLE run_events_p%s PARTITION OF run_events_partitioned FOR VALUES FROM (%L) TO (%L)',
        to_char(curr_start, 'YYYY_MM'), curr_start, next_start
    );
    EXECUTE format(
        'CREATE TABLE run_events_p%s PARTITION OF run_events_partitioned FOR VALUES FROM (%L) TO (%L)',
        to_char(next_start, 'YYYY_MM'), next_start, after_start
    );
END $body$;
-- +goose StatementEnd

-- 4. 分区本地索引（自动继承到每个分区）
CREATE INDEX ix_run_events_part_run_seq ON run_events_partitioned (run_id, seq);
CREATE INDEX ix_run_events_part_type ON run_events_partitioned (type);
CREATE INDEX ix_run_events_part_ts ON run_events_partitioned (ts);

-- 5. 从旧表迁移数据
INSERT INTO run_events_partitioned (event_id, run_id, seq, ts, type, data_json, tool_name, error_class)
SELECT event_id, run_id, seq, ts, type, data_json, tool_name, error_class
FROM run_events;

-- 6. 删除旧表，重命名分区表
DROP TABLE run_events;
ALTER TABLE run_events_partitioned RENAME TO run_events;

-- 7. 重命名约束和索引保持命名一致
ALTER TABLE run_events RENAME CONSTRAINT pk_run_events_part TO pk_run_events;
ALTER TABLE run_events RENAME CONSTRAINT uq_run_events_part_run_id_seq TO uq_run_events_run_id_seq;
ALTER INDEX ix_run_events_part_run_seq RENAME TO ix_run_events_run_seq;
ALTER INDEX ix_run_events_part_type RENAME TO ix_run_events_type;
ALTER INDEX ix_run_events_part_ts RENAME TO ix_run_events_ts;

-- +goose Down

-- 重建非分区表
CREATE TABLE run_events_flat (
    event_id    UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    run_id      UUID NOT NULL REFERENCES runs(id) ON DELETE CASCADE,
    seq         BIGINT NOT NULL DEFAULT nextval('run_events_seq_global'),
    ts          TIMESTAMPTZ NOT NULL DEFAULT now(),
    type        TEXT NOT NULL,
    data_json   JSONB NOT NULL DEFAULT '{}'::jsonb,
    tool_name   TEXT,
    error_class TEXT,
    CONSTRAINT uq_run_events_flat_run_id_seq UNIQUE (run_id, seq)
);

-- 拷贝数据
INSERT INTO run_events_flat (event_id, run_id, seq, ts, type, data_json, tool_name, error_class)
SELECT event_id, run_id, seq, ts, type, data_json, tool_name, error_class
FROM run_events;

-- 删除分区表，重命名为原始表名
DROP TABLE run_events;
ALTER TABLE run_events_flat RENAME TO run_events;
ALTER TABLE run_events RENAME CONSTRAINT uq_run_events_flat_run_id_seq TO uq_run_events_run_id_seq;

-- 重建原始索引
CREATE INDEX ix_run_events_run_seq ON run_events(run_id, seq);
CREATE INDEX ix_run_events_type ON run_events(type);
CREATE INDEX ix_run_events_tool_name ON run_events(tool_name);
CREATE INDEX ix_run_events_error_class ON run_events(error_class);
