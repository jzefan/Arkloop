-- +goose Up

-- light_context 和 tools_allow 是过度设计字段，pipeline 从未消费，直接移除
ALTER TABLE scheduled_jobs DROP COLUMN light_context;
ALTER TABLE scheduled_jobs DROP COLUMN tools_allow;

-- +goose Down

ALTER TABLE scheduled_jobs ADD COLUMN light_context BOOLEAN NOT NULL DEFAULT false;
ALTER TABLE scheduled_jobs ADD COLUMN tools_allow TEXT NOT NULL DEFAULT '';
