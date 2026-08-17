-- Local authentication replaces the alpha bootstrap/PAT flow. The old tables
-- remain for migration history only; every active legacy token is revoked and
-- the HTTP layer no longer authenticates against them.

CREATE TABLE identity.local_accounts (
    organization_id uuid NOT NULL,
    principal_id uuid NOT NULL,
    email text NOT NULL,
    username text NOT NULL,
    password_hash text NOT NULL,
    status text NOT NULL DEFAULT 'active',
    failed_login_attempts integer NOT NULL DEFAULT 0,
    locked_until timestamptz,
    password_changed_at timestamptz NOT NULL DEFAULT transaction_timestamp(),
    created_at timestamptz NOT NULL DEFAULT transaction_timestamp(),
    updated_at timestamptz NOT NULL DEFAULT transaction_timestamp(),
    CONSTRAINT local_accounts_principal_fk
        FOREIGN KEY (organization_id, principal_id)
        REFERENCES identity.principals (organization_id, id) ON DELETE RESTRICT,
    CONSTRAINT local_accounts_email_ck CHECK (
        email = lower(btrim(email))
        AND btrim(email) <> ''
        AND char_length(email) <= 320
    ),
    CONSTRAINT local_accounts_username_ck CHECK (
        username = lower(btrim(username))
        AND username ~ '^[a-z0-9][a-z0-9._-]{2,31}$'
    ),
    CONSTRAINT local_accounts_password_hash_ck CHECK (
        password_hash LIKE '$argon2id$%'
        AND char_length(password_hash) <= 512
    ),
    CONSTRAINT local_accounts_status_ck CHECK (
        status IN ('active', 'disabled')
    ),
    CONSTRAINT local_accounts_failed_attempts_ck CHECK (
        failed_login_attempts >= 0
    ),
    CONSTRAINT local_accounts_pk PRIMARY KEY (organization_id, principal_id)
);

CREATE UNIQUE INDEX local_accounts_email_uq
    ON identity.local_accounts (organization_id, lower(email));

CREATE UNIQUE INDEX local_accounts_username_uq
    ON identity.local_accounts (organization_id, lower(username));

CREATE TABLE identity.web_sessions (
    id uuid PRIMARY KEY DEFAULT uuidv7(),
    organization_id uuid NOT NULL,
    principal_id uuid NOT NULL,
    session_digest bytea NOT NULL,
    csrf_digest bytea NOT NULL,
    status text NOT NULL DEFAULT 'active',
    user_agent text NOT NULL DEFAULT '',
    remote_address text NOT NULL DEFAULT '',
    expires_at timestamptz NOT NULL,
    last_used_at timestamptz NOT NULL DEFAULT transaction_timestamp(),
    created_at timestamptz NOT NULL DEFAULT transaction_timestamp(),
    revoked_at timestamptz,
    CONSTRAINT web_sessions_principal_fk
        FOREIGN KEY (organization_id, principal_id)
        REFERENCES identity.principals (organization_id, id) ON DELETE RESTRICT,
    CONSTRAINT web_sessions_session_digest_ck CHECK (
        octet_length(session_digest) = 32
    ),
    CONSTRAINT web_sessions_csrf_digest_ck CHECK (
        octet_length(csrf_digest) = 32
    ),
    CONSTRAINT web_sessions_status_ck CHECK (
        status IN ('active', 'revoked', 'expired')
    ),
    CONSTRAINT web_sessions_expiry_ck CHECK (expires_at > created_at),
    CONSTRAINT web_sessions_revocation_ck CHECK (
        (status = 'revoked' AND revoked_at IS NOT NULL)
        OR (status <> 'revoked' AND revoked_at IS NULL)
    ),
    CONSTRAINT web_sessions_session_digest_uq UNIQUE (session_digest),
    CONSTRAINT web_sessions_tenant_id_uq UNIQUE (organization_id, id)
);

CREATE INDEX web_sessions_principal_idx
    ON identity.web_sessions (organization_id, principal_id, status, expires_at);

CREATE TABLE identity.device_authorizations (
    id uuid PRIMARY KEY DEFAULT uuidv7(),
    organization_id uuid NOT NULL,
    principal_id uuid,
    device_name text NOT NULL,
    device_code_digest bytea NOT NULL,
    user_code_digest bytea NOT NULL,
    user_code_hint text NOT NULL,
    status text NOT NULL DEFAULT 'pending',
    expires_at timestamptz NOT NULL,
    created_at timestamptz NOT NULL DEFAULT transaction_timestamp(),
    approved_at timestamptz,
    consumed_at timestamptz,
    CONSTRAINT device_authorizations_organization_fk
        FOREIGN KEY (organization_id)
        REFERENCES identity.organizations (id) ON DELETE RESTRICT,
    CONSTRAINT device_authorizations_principal_fk
        FOREIGN KEY (organization_id, principal_id)
        REFERENCES identity.principals (organization_id, id) ON DELETE RESTRICT,
    CONSTRAINT device_authorizations_name_ck CHECK (
        btrim(device_name) <> '' AND char_length(device_name) <= 200
    ),
    CONSTRAINT device_authorizations_device_digest_ck CHECK (
        octet_length(device_code_digest) = 32
    ),
    CONSTRAINT device_authorizations_user_digest_ck CHECK (
        octet_length(user_code_digest) = 32
    ),
    CONSTRAINT device_authorizations_hint_ck CHECK (
        user_code_hint ~ '^[A-Z0-9]{4}-[A-Z0-9]{4}$'
    ),
    CONSTRAINT device_authorizations_status_ck CHECK (
        status IN ('pending', 'approved', 'denied', 'consumed', 'expired')
    ),
    CONSTRAINT device_authorizations_expiry_ck CHECK (expires_at > created_at),
    CONSTRAINT device_authorizations_state_ck CHECK (
        (status = 'pending' AND principal_id IS NULL AND approved_at IS NULL AND consumed_at IS NULL)
        OR (status = 'approved' AND principal_id IS NOT NULL AND approved_at IS NOT NULL AND consumed_at IS NULL)
        OR (status = 'consumed' AND principal_id IS NOT NULL AND approved_at IS NOT NULL AND consumed_at IS NOT NULL)
        OR (status IN ('denied', 'expired') AND consumed_at IS NULL)
    ),
    CONSTRAINT device_authorizations_device_digest_uq UNIQUE (device_code_digest),
    CONSTRAINT device_authorizations_user_digest_uq UNIQUE (user_code_digest),
    CONSTRAINT device_authorizations_tenant_id_uq UNIQUE (organization_id, id)
);

CREATE INDEX device_authorizations_pending_idx
    ON identity.device_authorizations (organization_id, status, expires_at)
    WHERE status IN ('pending', 'approved');

CREATE TABLE identity.device_credentials (
    id uuid PRIMARY KEY DEFAULT uuidv7(),
    organization_id uuid NOT NULL,
    principal_id uuid NOT NULL,
    name text NOT NULL,
    credential_digest bytea NOT NULL,
    status text NOT NULL DEFAULT 'active',
    expires_at timestamptz NOT NULL,
    last_used_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT transaction_timestamp(),
    revoked_at timestamptz,
    CONSTRAINT device_credentials_principal_fk
        FOREIGN KEY (organization_id, principal_id)
        REFERENCES identity.principals (organization_id, id) ON DELETE RESTRICT,
    CONSTRAINT device_credentials_name_ck CHECK (
        btrim(name) <> '' AND char_length(name) <= 200
    ),
    CONSTRAINT device_credentials_digest_ck CHECK (
        octet_length(credential_digest) = 32
    ),
    CONSTRAINT device_credentials_status_ck CHECK (
        status IN ('active', 'revoked', 'expired')
    ),
    CONSTRAINT device_credentials_expiry_ck CHECK (expires_at > created_at),
    CONSTRAINT device_credentials_revocation_ck CHECK (
        (status = 'revoked' AND revoked_at IS NOT NULL)
        OR (status <> 'revoked' AND revoked_at IS NULL)
    ),
    CONSTRAINT device_credentials_digest_uq UNIQUE (credential_digest),
    CONSTRAINT device_credentials_tenant_id_uq UNIQUE (organization_id, id)
);

CREATE INDEX device_credentials_principal_idx
    ON identity.device_credentials (organization_id, principal_id, status, expires_at);

-- A leaked alpha credential must stop working immediately after this migration.
UPDATE identity.access_tokens
SET status = 'revoked', revoked_at = transaction_timestamp()
WHERE status = 'active';

-- The synthetic bootstrap principal remains only as historical attribution for
-- rows that reference it. It no longer owns the organization or any project.
DELETE FROM identity.project_memberships
WHERE principal_id = '00000000-0000-4000-8000-000000000002'::uuid;

DELETE FROM identity.organization_memberships
WHERE principal_id = '00000000-0000-4000-8000-000000000002'::uuid;

UPDATE identity.actors
SET status = 'disabled',
    attributes = attributes || '{"legacy_bootstrap": true}'::jsonb,
    updated_at = transaction_timestamp()
WHERE id = '00000000-0000-4000-8000-000000000002'::uuid;
