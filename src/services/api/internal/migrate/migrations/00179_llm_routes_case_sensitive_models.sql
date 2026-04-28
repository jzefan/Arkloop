-- +goose Up

DROP INDEX IF EXISTS ux_llm_routes_credential_model_lower;

CREATE UNIQUE INDEX ux_llm_routes_credential_model
    ON llm_routes (credential_id, model);

-- +goose Down

DROP INDEX IF EXISTS ux_llm_routes_credential_model;

CREATE UNIQUE INDEX ux_llm_routes_credential_model_lower
    ON llm_routes (credential_id, lower(model));
