-- +goose Up
CREATE TABLE org_invitations (
    id                  UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id              UUID        NOT NULL REFERENCES orgs(id) ON DELETE CASCADE,
    invited_by_user_id  UUID        NOT NULL REFERENCES users(id),
    email               TEXT        NOT NULL,
    role                TEXT        NOT NULL DEFAULT 'member',
    token               TEXT        NOT NULL UNIQUE,
    expires_at          TIMESTAMPTZ NOT NULL,
    accepted_at         TIMESTAMPTZ,
    accepted_by_user_id UUID        REFERENCES users(id),
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_org_invitations_org_id ON org_invitations(org_id) WHERE accepted_at IS NULL;
CREATE INDEX idx_org_invitations_token  ON org_invitations(token);

-- +goose Down
DROP TABLE IF EXISTS org_invitations;
