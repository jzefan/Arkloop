-- +goose Up

-- 用 reasoning_mode 字符串字段替代 thinking 布尔字段，与聊天 run.started 链路对齐
ALTER TABLE scheduled_jobs ADD COLUMN reasoning_mode TEXT NOT NULL DEFAULT '';
ALTER TABLE scheduled_jobs DROP COLUMN thinking;

-- +goose Down

ALTER TABLE scheduled_jobs ADD COLUMN thinking BOOLEAN NOT NULL DEFAULT false;
ALTER TABLE scheduled_jobs DROP COLUMN reasoning_mode;
