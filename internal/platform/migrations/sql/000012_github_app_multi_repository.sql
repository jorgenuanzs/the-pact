CREATE SCHEMA integrations;

CREATE TABLE integrations.github_installations (
    organization_id uuid NOT NULL,
    installation_id bigint NOT NULL,
    account_id bigint NOT NULL,
    account_login text NOT NULL,
    account_type text NOT NULL,
    repository_selection text NOT NULL,
    permissions jsonb NOT NULL DEFAULT '{}'::jsonb,
    status text NOT NULL DEFAULT 'active',
    installed_at timestamptz NOT NULL DEFAULT transaction_timestamp(),
    suspended_at timestamptz,
    deleted_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT transaction_timestamp(),
    updated_at timestamptz NOT NULL DEFAULT transaction_timestamp(),
    version bigint NOT NULL DEFAULT 1,
    CONSTRAINT github_installations_organization_fk
        FOREIGN KEY (organization_id)
        REFERENCES identity.organizations (id)
        ON DELETE RESTRICT,
    CONSTRAINT github_installations_id_ck CHECK (installation_id > 0),
    CONSTRAINT github_installations_account_id_ck CHECK (account_id > 0),
    CONSTRAINT github_installations_account_login_ck CHECK (
        btrim(account_login) <> '' AND char_length(account_login) <= 255
    ),
    CONSTRAINT github_installations_account_type_ck CHECK (
        account_type IN ('Organization', 'User', 'Bot')
    ),
    CONSTRAINT github_installations_selection_ck CHECK (
        repository_selection IN ('all', 'selected')
    ),
    CONSTRAINT github_installations_permissions_shape_ck CHECK (
        jsonb_typeof(permissions) = 'object'
    ),
    CONSTRAINT github_installations_status_ck CHECK (
        status IN ('active', 'suspended', 'deleted')
    ),
    CONSTRAINT github_installations_state_ck CHECK (
        (status = 'active' AND suspended_at IS NULL AND deleted_at IS NULL)
        OR (status = 'suspended' AND suspended_at IS NOT NULL AND deleted_at IS NULL)
        OR (status = 'deleted' AND deleted_at IS NOT NULL)
    ),
    CONSTRAINT github_installations_version_ck CHECK (version > 0),
    CONSTRAINT github_installations_pk PRIMARY KEY (organization_id, installation_id)
);

CREATE INDEX github_installations_status_idx
    ON integrations.github_installations (organization_id, status, updated_at DESC);

CREATE TABLE integrations.github_repositories (
    organization_id uuid NOT NULL,
    github_repository_id bigint NOT NULL,
    installation_id bigint NOT NULL,
    owner_login text NOT NULL,
    name text NOT NULL,
    full_name text NOT NULL,
    private boolean NOT NULL,
    visibility text NOT NULL,
    default_branch text NOT NULL,
    html_url text NOT NULL,
    clone_url text NOT NULL,
    status text NOT NULL DEFAULT 'active',
    provider_updated_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT transaction_timestamp(),
    updated_at timestamptz NOT NULL DEFAULT transaction_timestamp(),
    version bigint NOT NULL DEFAULT 1,
    CONSTRAINT github_repositories_installation_fk
        FOREIGN KEY (organization_id, installation_id)
        REFERENCES integrations.github_installations (organization_id, installation_id)
        ON DELETE RESTRICT,
    CONSTRAINT github_repositories_id_ck CHECK (github_repository_id > 0),
    CONSTRAINT github_repositories_owner_ck CHECK (
        btrim(owner_login) <> '' AND char_length(owner_login) <= 255
    ),
    CONSTRAINT github_repositories_name_ck CHECK (
        btrim(name) <> '' AND char_length(name) <= 255
    ),
    CONSTRAINT github_repositories_full_name_ck CHECK (
        btrim(full_name) <> '' AND char_length(full_name) <= 511
    ),
    CONSTRAINT github_repositories_visibility_ck CHECK (
        visibility IN ('public', 'private', 'internal')
    ),
    CONSTRAINT github_repositories_default_branch_ck CHECK (
        btrim(default_branch) <> '' AND char_length(default_branch) <= 255
    ),
    CONSTRAINT github_repositories_html_url_ck CHECK (
        btrim(html_url) <> '' AND char_length(html_url) <= 2048
    ),
    CONSTRAINT github_repositories_clone_url_ck CHECK (
        btrim(clone_url) <> '' AND char_length(clone_url) <= 2048
    ),
    CONSTRAINT github_repositories_status_ck CHECK (
        status IN ('active', 'unavailable', 'removed')
    ),
    CONSTRAINT github_repositories_version_ck CHECK (version > 0),
    CONSTRAINT github_repositories_pk PRIMARY KEY (organization_id, github_repository_id),
    CONSTRAINT github_repositories_full_name_uq UNIQUE (organization_id, full_name)
);

CREATE INDEX github_repositories_installation_status_idx
    ON integrations.github_repositories (
        organization_id,
        installation_id,
        status,
        full_name
    );

CREATE TABLE integrations.github_connection_attempts (
    id uuid PRIMARY KEY DEFAULT uuidv7(),
    organization_id uuid NOT NULL,
    principal_id uuid NOT NULL,
    state_digest bytea NOT NULL,
    installation_id bigint,
    status text NOT NULL DEFAULT 'pending',
    expires_at timestamptz NOT NULL,
    consumed_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT transaction_timestamp(),
    CONSTRAINT github_connection_attempts_principal_fk
        FOREIGN KEY (organization_id, principal_id)
        REFERENCES identity.principals (organization_id, id)
        ON DELETE RESTRICT,
    CONSTRAINT github_connection_attempts_digest_ck CHECK (octet_length(state_digest) = 32),
    CONSTRAINT github_connection_attempts_installation_id_ck CHECK (
        installation_id IS NULL OR installation_id > 0
    ),
    CONSTRAINT github_connection_attempts_status_ck CHECK (
        status IN ('pending', 'completed', 'failed')
    ),
    CONSTRAINT github_connection_attempts_state_ck CHECK (
        (status = 'pending' AND consumed_at IS NULL)
        OR (status <> 'pending' AND consumed_at IS NOT NULL AND installation_id IS NOT NULL)
    ),
    CONSTRAINT github_connection_attempts_state_uq UNIQUE (state_digest)
);

CREATE INDEX github_connection_attempts_expiry_idx
    ON integrations.github_connection_attempts (organization_id, status, expires_at);

CREATE TABLE integrations.github_webhook_deliveries (
    organization_id uuid NOT NULL,
    delivery_id text NOT NULL,
    event_type text NOT NULL,
    payload_hash bytea NOT NULL,
    status text NOT NULL DEFAULT 'processing',
    error_code text,
    received_at timestamptz NOT NULL DEFAULT transaction_timestamp(),
    processed_at timestamptz,
    CONSTRAINT github_webhook_deliveries_organization_fk
        FOREIGN KEY (organization_id)
        REFERENCES identity.organizations (id)
        ON DELETE RESTRICT,
    CONSTRAINT github_webhook_deliveries_id_ck CHECK (
        btrim(delivery_id) <> '' AND char_length(delivery_id) <= 255
    ),
    CONSTRAINT github_webhook_deliveries_event_ck CHECK (
        btrim(event_type) <> '' AND char_length(event_type) <= 100
    ),
    CONSTRAINT github_webhook_deliveries_hash_ck CHECK (octet_length(payload_hash) = 32),
    CONSTRAINT github_webhook_deliveries_status_ck CHECK (
        status IN ('processing', 'processed', 'failed')
    ),
    CONSTRAINT github_webhook_deliveries_state_ck CHECK (
        (status = 'processing' AND processed_at IS NULL)
        OR (status <> 'processing' AND processed_at IS NOT NULL)
    ),
    CONSTRAINT github_webhook_deliveries_pk PRIMARY KEY (organization_id, delivery_id)
);

ALTER TABLE coordination.repositories
    ADD COLUMN github_repository_id bigint,
    ADD COLUMN purpose text NOT NULL DEFAULT 'application',
    ADD COLUMN is_required boolean NOT NULL DEFAULT true,
    ADD CONSTRAINT repositories_github_repository_fk
        FOREIGN KEY (organization_id, github_repository_id)
        REFERENCES integrations.github_repositories (organization_id, github_repository_id)
        ON DELETE RESTRICT,
    ADD CONSTRAINT repositories_purpose_ck CHECK (
        char_length(purpose) BETWEEN 1 AND 64
        AND purpose ~ '^[a-z][a-z0-9_-]*$'
    );

UPDATE coordination.repositories AS repository
SET purpose = 'primary'
FROM identity.projects AS project
WHERE project.organization_id = repository.organization_id
  AND project.id = repository.project_id
  AND project.root_repository_id = repository.id;

CREATE UNIQUE INDEX repositories_project_github_uq
    ON coordination.repositories (organization_id, project_id, github_repository_id)
    WHERE github_repository_id IS NOT NULL AND status <> 'archived';

CREATE INDEX repositories_project_purpose_idx
    ON coordination.repositories (organization_id, project_id, purpose, status);
