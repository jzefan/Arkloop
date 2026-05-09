-- +goose Up

ALTER TABLE llm_credentials
    DROP CONSTRAINT IF EXISTS llm_credentials_provider_check;

ALTER TABLE llm_credentials
    ADD CONSTRAINT llm_credentials_provider_check
        CHECK (provider IN ('openai', 'anthropic', 'gemini', 'deepseek', 'doubao', 'qwen', 'yuanbao', 'kimi', 'zenmax'));

-- +goose Down

ALTER TABLE llm_credentials
    DROP CONSTRAINT IF EXISTS llm_credentials_provider_check;

ALTER TABLE llm_credentials
    ADD CONSTRAINT llm_credentials_provider_check
        CHECK (provider IN ('openai', 'anthropic', 'gemini', 'deepseek', 'doubao', 'qwen', 'zenmax'));
