-- +goose Up
ALTER TABLE webhook_endpoints
    ADD COLUMN secret_id UUID REFERENCES secrets(id) ON DELETE SET NULL;

ALTER TABLE webhook_endpoints
    ALTER COLUMN signing_secret DROP NOT NULL;

CREATE INDEX idx_webhook_endpoints_secret_id ON webhook_endpoints(secret_id);

-- +goose Down
DROP INDEX IF EXISTS idx_webhook_endpoints_secret_id;

ALTER TABLE webhook_endpoints
    ALTER COLUMN signing_secret SET NOT NULL;

ALTER TABLE webhook_endpoints
    DROP COLUMN IF EXISTS secret_id;
