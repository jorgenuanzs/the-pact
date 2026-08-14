CREATE TABLE coordination.repository_provider_states (
    repository_id uuid PRIMARY KEY,
    organization_id uuid NOT NULL,
    project_id uuid NOT NULL,
    provider text NOT NULL,
    repository_full_name text NOT NULL,
    status text NOT NULL,
    default_branch text NOT NULL,
    canonical_revision text,
    visibility text NOT NULL DEFAULT 'unknown',
    provider_updated_at timestamptz,
    last_attempt_at timestamptz NOT NULL DEFAULT transaction_timestamp(),
    last_success_at timestamptz,
    last_error_code text,
    version bigint NOT NULL DEFAULT 1,
    created_at timestamptz NOT NULL DEFAULT transaction_timestamp(),
    updated_at timestamptz NOT NULL DEFAULT transaction_timestamp(),
    CONSTRAINT repository_provider_states_repository_fk
        FOREIGN KEY (organization_id, project_id, repository_id)
        REFERENCES coordination.repositories (organization_id, project_id, id)
        ON DELETE CASCADE,
    CONSTRAINT repository_provider_states_provider_ck CHECK (
        provider IN ('github')
    ),
    CONSTRAINT repository_provider_states_full_name_ck CHECK (
        btrim(repository_full_name) <> ''
        AND char_length(repository_full_name) <= 202
    ),
    CONSTRAINT repository_provider_states_status_ck CHECK (
        status IN ('synced', 'failed')
    ),
    CONSTRAINT repository_provider_states_default_branch_ck CHECK (
        btrim(default_branch) <> ''
        AND char_length(default_branch) <= 255
    ),
    CONSTRAINT repository_provider_states_revision_ck CHECK (
        canonical_revision IS NULL
        OR canonical_revision ~ '^[0-9a-f]{7,64}$'
    ),
    CONSTRAINT repository_provider_states_visibility_ck CHECK (
        visibility IN ('public', 'private', 'internal', 'unknown')
    ),
    CONSTRAINT repository_provider_states_error_code_ck CHECK (
        last_error_code IS NULL
        OR (
            last_error_code ~ '^[a-z][a-z0-9_]*$'
            AND char_length(last_error_code) <= 100
        )
    ),
    CONSTRAINT repository_provider_states_success_state_ck CHECK (
        (
            status = 'synced'
            AND canonical_revision IS NOT NULL
            AND last_success_at IS NOT NULL
            AND last_error_code IS NULL
        )
        OR (
            status = 'failed'
            AND last_error_code IS NOT NULL
        )
    ),
    CONSTRAINT repository_provider_states_time_order_ck CHECK (
        last_success_at IS NULL OR last_success_at <= last_attempt_at
    ),
    CONSTRAINT repository_provider_states_version_ck CHECK (version > 0),
    CONSTRAINT repository_provider_states_tenant_id_uq
        UNIQUE (organization_id, project_id, repository_id)
);

CREATE INDEX repository_provider_states_project_status_idx
    ON coordination.repository_provider_states (
        organization_id, project_id, status, updated_at DESC
    );
