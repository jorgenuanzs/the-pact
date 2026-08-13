CREATE TABLE identity.organization_memberships (
    organization_id uuid NOT NULL,
    principal_id uuid NOT NULL,
    role text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT transaction_timestamp(),
    updated_at timestamptz NOT NULL DEFAULT transaction_timestamp(),
    CONSTRAINT organization_memberships_organization_fk
        FOREIGN KEY (organization_id)
        REFERENCES identity.organizations (id) ON DELETE RESTRICT,
    CONSTRAINT organization_memberships_principal_fk
        FOREIGN KEY (organization_id, principal_id)
        REFERENCES identity.principals (organization_id, id) ON DELETE RESTRICT,
    CONSTRAINT organization_memberships_role_ck CHECK (
        role IN ('owner', 'admin', 'member')
    ),
    CONSTRAINT organization_memberships_pk
        PRIMARY KEY (organization_id, principal_id)
);

CREATE TABLE identity.project_memberships (
    organization_id uuid NOT NULL,
    project_id uuid NOT NULL,
    principal_id uuid NOT NULL,
    role text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT transaction_timestamp(),
    updated_at timestamptz NOT NULL DEFAULT transaction_timestamp(),
    CONSTRAINT project_memberships_project_fk
        FOREIGN KEY (organization_id, project_id)
        REFERENCES identity.projects (organization_id, id) ON DELETE RESTRICT,
    CONSTRAINT project_memberships_principal_fk
        FOREIGN KEY (organization_id, principal_id)
        REFERENCES identity.principals (organization_id, id) ON DELETE RESTRICT,
    CONSTRAINT project_memberships_role_ck CHECK (
        role IN ('owner', 'maintainer', 'contributor', 'viewer')
    ),
    CONSTRAINT project_memberships_pk
        PRIMARY KEY (organization_id, project_id, principal_id)
);

CREATE INDEX project_memberships_principal_idx
    ON identity.project_memberships (organization_id, principal_id, project_id);

CREATE TABLE identity.invitations (
    id uuid PRIMARY KEY DEFAULT uuidv7(),
    organization_id uuid NOT NULL,
    project_id uuid NOT NULL,
    email text NOT NULL,
    role text NOT NULL,
    token_digest bytea NOT NULL,
    status text NOT NULL DEFAULT 'pending',
    expires_at timestamptz NOT NULL,
    created_by_principal_id uuid NOT NULL,
    accepted_by_principal_id uuid,
    created_at timestamptz NOT NULL DEFAULT transaction_timestamp(),
    accepted_at timestamptz,
    revoked_at timestamptz,
    CONSTRAINT invitations_project_fk
        FOREIGN KEY (organization_id, project_id)
        REFERENCES identity.projects (organization_id, id) ON DELETE RESTRICT,
    CONSTRAINT invitations_creator_fk
        FOREIGN KEY (organization_id, created_by_principal_id)
        REFERENCES identity.principals (organization_id, id) ON DELETE RESTRICT,
    CONSTRAINT invitations_acceptor_fk
        FOREIGN KEY (organization_id, accepted_by_principal_id)
        REFERENCES identity.principals (organization_id, id) ON DELETE RESTRICT,
    CONSTRAINT invitations_email_ck CHECK (
        btrim(email) <> '' AND char_length(email) <= 320
    ),
    CONSTRAINT invitations_role_ck CHECK (
        role IN ('owner', 'maintainer', 'contributor', 'viewer')
    ),
    CONSTRAINT invitations_token_digest_ck CHECK (
        octet_length(token_digest) = 32
    ),
    CONSTRAINT invitations_status_ck CHECK (
        status IN ('pending', 'accepted', 'revoked', 'expired')
    ),
    CONSTRAINT invitations_expiry_ck CHECK (expires_at > created_at),
    CONSTRAINT invitations_state_ck CHECK (
        (status = 'pending' AND accepted_by_principal_id IS NULL AND accepted_at IS NULL AND revoked_at IS NULL)
        OR (status = 'accepted' AND accepted_by_principal_id IS NOT NULL AND accepted_at IS NOT NULL AND revoked_at IS NULL)
        OR (status = 'revoked' AND accepted_by_principal_id IS NULL AND accepted_at IS NULL AND revoked_at IS NOT NULL)
        OR (status = 'expired' AND accepted_by_principal_id IS NULL AND accepted_at IS NULL AND revoked_at IS NULL)
    ),
    CONSTRAINT invitations_token_digest_uq UNIQUE (token_digest),
    CONSTRAINT invitations_tenant_id_uq UNIQUE (organization_id, id)
);

CREATE UNIQUE INDEX invitations_pending_email_uq
    ON identity.invitations (organization_id, project_id, lower(email))
    WHERE status = 'pending';

CREATE INDEX invitations_expiry_idx
    ON identity.invitations (status, expires_at)
    WHERE status = 'pending';

CREATE TABLE identity.access_tokens (
    id uuid PRIMARY KEY DEFAULT uuidv7(),
    organization_id uuid NOT NULL,
    principal_id uuid NOT NULL,
    name text NOT NULL,
    token_digest bytea NOT NULL,
    status text NOT NULL DEFAULT 'active',
    expires_at timestamptz NOT NULL,
    last_used_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT transaction_timestamp(),
    revoked_at timestamptz,
    CONSTRAINT access_tokens_principal_fk
        FOREIGN KEY (organization_id, principal_id)
        REFERENCES identity.principals (organization_id, id) ON DELETE RESTRICT,
    CONSTRAINT access_tokens_name_ck CHECK (
        btrim(name) <> '' AND char_length(name) <= 200
    ),
    CONSTRAINT access_tokens_token_digest_ck CHECK (
        octet_length(token_digest) = 32
    ),
    CONSTRAINT access_tokens_status_ck CHECK (
        status IN ('active', 'revoked', 'expired')
    ),
    CONSTRAINT access_tokens_expiry_ck CHECK (expires_at > created_at),
    CONSTRAINT access_tokens_revocation_ck CHECK (
        (status = 'revoked' AND revoked_at IS NOT NULL)
        OR (status <> 'revoked' AND revoked_at IS NULL)
    ),
    CONSTRAINT access_tokens_token_digest_uq UNIQUE (token_digest),
    CONSTRAINT access_tokens_tenant_id_uq UNIQUE (organization_id, id)
);

CREATE INDEX access_tokens_principal_idx
    ON identity.access_tokens (organization_id, principal_id, status, expires_at);

INSERT INTO identity.organization_memberships (
    organization_id,
    principal_id,
    role
) VALUES (
    '00000000-0000-4000-8000-000000000001'::uuid,
    '00000000-0000-4000-8000-000000000002'::uuid,
    'owner'
);

INSERT INTO identity.project_memberships (
    organization_id,
    project_id,
    principal_id,
    role
)
SELECT
    organization_id,
    id,
    '00000000-0000-4000-8000-000000000002'::uuid,
    'owner'
FROM identity.projects
WHERE status <> 'archived';
