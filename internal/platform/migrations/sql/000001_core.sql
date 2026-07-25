CREATE EXTENSION IF NOT EXISTS vector;

CREATE SCHEMA IF NOT EXISTS identity;
CREATE SCHEMA IF NOT EXISTS coordination;
CREATE SCHEMA IF NOT EXISTS platform;

SET LOCAL TIME ZONE 'UTC';

-- ---------------------------------------------------------------------------
-- Identity and tenancy
-- ---------------------------------------------------------------------------

CREATE TABLE identity.organizations (
    id uuid PRIMARY KEY DEFAULT uuidv7(),
    slug text NOT NULL,
    name text NOT NULL,
    status text NOT NULL DEFAULT 'active',
    settings jsonb NOT NULL DEFAULT '{}'::jsonb,
    version bigint NOT NULL DEFAULT 1,
    created_at timestamptz NOT NULL DEFAULT transaction_timestamp(),
    updated_at timestamptz NOT NULL DEFAULT transaction_timestamp(),
    archived_at timestamptz,
    CONSTRAINT organizations_slug_format_ck CHECK (
        char_length(slug) BETWEEN 1 AND 63
        AND slug ~ '^[a-z0-9][a-z0-9-]*$'
        AND slug !~ '-$'
    ),
    CONSTRAINT organizations_name_ck CHECK (
        btrim(name) <> ''
        AND char_length(name) <= 200
    ),
    CONSTRAINT organizations_status_ck CHECK (
        status IN ('active', 'suspended', 'archived')
    ),
    CONSTRAINT organizations_settings_shape_ck CHECK (
        jsonb_typeof(settings) = 'object'
    ),
    CONSTRAINT organizations_version_ck CHECK (version > 0),
    CONSTRAINT organizations_archive_state_ck CHECK (
        (status = 'archived' AND archived_at IS NOT NULL)
        OR (status <> 'archived' AND archived_at IS NULL)
    ),
    CONSTRAINT organizations_slug_uq UNIQUE (slug)
);

INSERT INTO identity.organizations (
    id,
    slug,
    name,
    status
) VALUES (
    '00000000-0000-4000-8000-000000000001'::uuid,
    'local',
    'Local',
    'active'
);

CREATE TABLE identity.projects (
    id uuid PRIMARY KEY DEFAULT uuidv7(),
    organization_id uuid NOT NULL,
    slug text NOT NULL,
    name text NOT NULL,
    status text NOT NULL DEFAULT 'initializing',
    root_repository_id uuid,
    canonical_revision text,
    event_sequence bigint NOT NULL DEFAULT 0,
    settings jsonb NOT NULL DEFAULT '{}'::jsonb,
    version bigint NOT NULL DEFAULT 1,
    created_at timestamptz NOT NULL DEFAULT transaction_timestamp(),
    updated_at timestamptz NOT NULL DEFAULT transaction_timestamp(),
    archived_at timestamptz,
    CONSTRAINT projects_organization_fk FOREIGN KEY (organization_id)
        REFERENCES identity.organizations (id) ON DELETE RESTRICT,
    CONSTRAINT projects_slug_format_ck CHECK (
        char_length(slug) BETWEEN 1 AND 63
        AND slug ~ '^[a-z0-9][a-z0-9-]*$'
        AND slug !~ '-$'
    ),
    CONSTRAINT projects_name_ck CHECK (
        btrim(name) <> ''
        AND char_length(name) <= 200
    ),
    CONSTRAINT projects_status_ck CHECK (
        status IN ('initializing', 'active', 'archived')
    ),
    CONSTRAINT projects_canonical_revision_ck CHECK (
        canonical_revision IS NULL
        OR (
            btrim(canonical_revision) <> ''
            AND char_length(canonical_revision) <= 255
        )
    ),
    CONSTRAINT projects_event_sequence_ck CHECK (event_sequence >= 0),
    CONSTRAINT projects_settings_shape_ck CHECK (
        jsonb_typeof(settings) = 'object'
    ),
    CONSTRAINT projects_version_ck CHECK (version > 0),
    CONSTRAINT projects_archive_state_ck CHECK (
        (status = 'archived' AND archived_at IS NOT NULL)
        OR (status <> 'archived' AND archived_at IS NULL)
    ),
    CONSTRAINT projects_tenant_id_uq UNIQUE (organization_id, id),
    CONSTRAINT projects_tenant_slug_uq UNIQUE (organization_id, slug)
);

CREATE INDEX projects_tenant_status_idx
    ON identity.projects (organization_id, status, updated_at DESC);

CREATE TABLE identity.actors (
    id uuid PRIMARY KEY DEFAULT uuidv7(),
    organization_id uuid NOT NULL,
    kind text NOT NULL,
    status text NOT NULL DEFAULT 'active',
    display_name text NOT NULL,
    attributes jsonb NOT NULL DEFAULT '{}'::jsonb,
    version bigint NOT NULL DEFAULT 1,
    created_at timestamptz NOT NULL DEFAULT transaction_timestamp(),
    updated_at timestamptz NOT NULL DEFAULT transaction_timestamp(),
    retired_at timestamptz,
    CONSTRAINT actors_organization_fk FOREIGN KEY (organization_id)
        REFERENCES identity.organizations (id) ON DELETE RESTRICT,
    CONSTRAINT actors_kind_ck CHECK (
        kind IN ('principal', 'agent', 'node', 'runner', 'connector', 'system')
    ),
    CONSTRAINT actors_status_ck CHECK (
        status IN ('active', 'disabled', 'retired')
    ),
    CONSTRAINT actors_display_name_ck CHECK (
        btrim(display_name) <> ''
        AND char_length(display_name) <= 200
    ),
    CONSTRAINT actors_attributes_shape_ck CHECK (
        jsonb_typeof(attributes) = 'object'
    ),
    CONSTRAINT actors_version_ck CHECK (version > 0),
    CONSTRAINT actors_retired_state_ck CHECK (
        (status = 'retired' AND retired_at IS NOT NULL)
        OR (status <> 'retired' AND retired_at IS NULL)
    ),
    CONSTRAINT actors_tenant_id_uq UNIQUE (organization_id, id),
    CONSTRAINT actors_tenant_kind_uq UNIQUE (organization_id, id, kind)
);

CREATE INDEX actors_tenant_kind_status_idx
    ON identity.actors (organization_id, kind, status);

CREATE TABLE identity.principals (
    id uuid PRIMARY KEY,
    organization_id uuid NOT NULL,
    actor_kind text NOT NULL DEFAULT 'principal',
    principal_type text NOT NULL,
    external_issuer text,
    external_subject text,
    created_at timestamptz NOT NULL DEFAULT transaction_timestamp(),
    updated_at timestamptz NOT NULL DEFAULT transaction_timestamp(),
    CONSTRAINT principals_actor_kind_ck CHECK (actor_kind = 'principal'),
    CONSTRAINT principals_type_ck CHECK (
        principal_type IN ('human', 'service')
    ),
    CONSTRAINT principals_external_identity_pair_ck CHECK (
        (external_issuer IS NULL AND external_subject IS NULL)
        OR (
            external_issuer IS NOT NULL
            AND external_subject IS NOT NULL
            AND btrim(external_issuer) <> ''
            AND btrim(external_subject) <> ''
        )
    ),
    CONSTRAINT principals_actor_fk
        FOREIGN KEY (organization_id, id, actor_kind)
        REFERENCES identity.actors (organization_id, id, kind)
        ON DELETE RESTRICT,
    CONSTRAINT principals_tenant_id_uq UNIQUE (organization_id, id)
);

CREATE UNIQUE INDEX principals_external_identity_uq
    ON identity.principals (
        organization_id,
        external_issuer,
        external_subject
    )
    WHERE external_issuer IS NOT NULL AND external_subject IS NOT NULL;

CREATE TABLE identity.agents (
    id uuid PRIMARY KEY,
    organization_id uuid NOT NULL,
    actor_kind text NOT NULL DEFAULT 'agent',
    sponsor_principal_id uuid NOT NULL,
    name text NOT NULL,
    agent_type text NOT NULL,
    declared_capabilities jsonb NOT NULL DEFAULT '{}'::jsonb,
    created_at timestamptz NOT NULL DEFAULT transaction_timestamp(),
    updated_at timestamptz NOT NULL DEFAULT transaction_timestamp(),
    CONSTRAINT agents_actor_kind_ck CHECK (actor_kind = 'agent'),
    CONSTRAINT agents_name_ck CHECK (
        btrim(name) <> ''
        AND char_length(name) <= 200
    ),
    CONSTRAINT agents_type_ck CHECK (
        btrim(agent_type) <> ''
        AND char_length(agent_type) <= 100
    ),
    CONSTRAINT agents_capabilities_shape_ck CHECK (
        jsonb_typeof(declared_capabilities) = 'object'
    ),
    CONSTRAINT agents_actor_fk
        FOREIGN KEY (organization_id, id, actor_kind)
        REFERENCES identity.actors (organization_id, id, kind)
        ON DELETE RESTRICT,
    CONSTRAINT agents_sponsor_fk
        FOREIGN KEY (organization_id, sponsor_principal_id)
        REFERENCES identity.principals (organization_id, id)
        ON DELETE RESTRICT,
    CONSTRAINT agents_tenant_id_uq UNIQUE (organization_id, id)
);

CREATE INDEX agents_tenant_sponsor_idx
    ON identity.agents (organization_id, sponsor_principal_id, created_at DESC);

CREATE TABLE identity.nodes (
    id uuid PRIMARY KEY,
    organization_id uuid NOT NULL,
    actor_kind text NOT NULL DEFAULT 'node',
    node_key text NOT NULL,
    name text NOT NULL,
    node_type text NOT NULL DEFAULT 'local',
    lifecycle_status text NOT NULL DEFAULT 'registered',
    labels jsonb NOT NULL DEFAULT '{}'::jsonb,
    capabilities jsonb NOT NULL DEFAULT '{}'::jsonb,
    version bigint NOT NULL DEFAULT 1,
    created_at timestamptz NOT NULL DEFAULT transaction_timestamp(),
    updated_at timestamptz NOT NULL DEFAULT transaction_timestamp(),
    last_seen_at timestamptz,
    revoked_at timestamptz,
    CONSTRAINT nodes_actor_kind_ck CHECK (actor_kind = 'node'),
    CONSTRAINT nodes_key_ck CHECK (
        btrim(node_key) <> ''
        AND char_length(node_key) <= 255
    ),
    CONSTRAINT nodes_name_ck CHECK (
        btrim(name) <> ''
        AND char_length(name) <= 200
    ),
    CONSTRAINT nodes_type_ck CHECK (
        node_type IN ('local', 'remote', 'runner_host')
    ),
    CONSTRAINT nodes_lifecycle_status_ck CHECK (
        lifecycle_status IN ('registered', 'active', 'offline', 'revoked')
    ),
    CONSTRAINT nodes_labels_shape_ck CHECK (
        jsonb_typeof(labels) = 'object'
    ),
    CONSTRAINT nodes_capabilities_shape_ck CHECK (
        jsonb_typeof(capabilities) = 'object'
    ),
    CONSTRAINT nodes_version_ck CHECK (version > 0),
    CONSTRAINT nodes_revoked_state_ck CHECK (
        (lifecycle_status = 'revoked' AND revoked_at IS NOT NULL)
        OR (lifecycle_status <> 'revoked' AND revoked_at IS NULL)
    ),
    CONSTRAINT nodes_actor_fk
        FOREIGN KEY (organization_id, id, actor_kind)
        REFERENCES identity.actors (organization_id, id, kind)
        ON DELETE RESTRICT,
    CONSTRAINT nodes_tenant_id_uq UNIQUE (organization_id, id),
    CONSTRAINT nodes_tenant_key_uq UNIQUE (organization_id, node_key)
);

CREATE INDEX nodes_tenant_status_idx
    ON identity.nodes (organization_id, lifecycle_status, updated_at DESC);

CREATE TABLE identity.sessions (
    id uuid PRIMARY KEY DEFAULT uuidv7(),
    organization_id uuid NOT NULL,
    project_id uuid NOT NULL,
    actor_id uuid NOT NULL,
    node_id uuid,
    status text NOT NULL DEFAULT 'starting',
    client_type text NOT NULL,
    protocol_version text NOT NULL,
    announced_capabilities jsonb NOT NULL DEFAULT '{}'::jsonb,
    token_digest bytea,
    version bigint NOT NULL DEFAULT 1,
    started_at timestamptz NOT NULL DEFAULT transaction_timestamp(),
    last_seen_at timestamptz NOT NULL DEFAULT transaction_timestamp(),
    expires_at timestamptz NOT NULL,
    ended_at timestamptz,
    CONSTRAINT sessions_project_fk
        FOREIGN KEY (organization_id, project_id)
        REFERENCES identity.projects (organization_id, id)
        ON DELETE RESTRICT,
    CONSTRAINT sessions_actor_fk
        FOREIGN KEY (organization_id, actor_id)
        REFERENCES identity.actors (organization_id, id)
        ON DELETE RESTRICT,
    CONSTRAINT sessions_node_fk
        FOREIGN KEY (organization_id, node_id)
        REFERENCES identity.nodes (organization_id, id)
        ON DELETE RESTRICT,
    CONSTRAINT sessions_status_ck CHECK (
        status IN ('starting', 'active', 'stale', 'closed', 'expired')
    ),
    CONSTRAINT sessions_client_type_ck CHECK (
        btrim(client_type) <> ''
        AND char_length(client_type) <= 100
    ),
    CONSTRAINT sessions_protocol_version_ck CHECK (
        btrim(protocol_version) <> ''
        AND char_length(protocol_version) <= 50
    ),
    CONSTRAINT sessions_capabilities_shape_ck CHECK (
        jsonb_typeof(announced_capabilities) = 'object'
    ),
    CONSTRAINT sessions_token_digest_ck CHECK (
        token_digest IS NULL OR octet_length(token_digest) = 32
    ),
    CONSTRAINT sessions_version_ck CHECK (version > 0),
    CONSTRAINT sessions_time_order_ck CHECK (
        last_seen_at >= started_at
        AND expires_at > started_at
        AND last_seen_at <= expires_at
    ),
    CONSTRAINT sessions_end_state_ck CHECK (
        (status IN ('closed', 'expired') AND ended_at IS NOT NULL)
        OR (status IN ('starting', 'active', 'stale') AND ended_at IS NULL)
    ),
    CONSTRAINT sessions_tenant_project_id_uq
        UNIQUE (organization_id, project_id, id),
    CONSTRAINT sessions_token_digest_uq UNIQUE (token_digest)
);

CREATE INDEX sessions_live_project_idx
    ON identity.sessions (
        organization_id,
        project_id,
        status,
        expires_at
    )
    WHERE status IN ('starting', 'active', 'stale');

CREATE INDEX sessions_live_actor_idx
    ON identity.sessions (organization_id, actor_id, status, expires_at)
    WHERE status IN ('starting', 'active', 'stale');

-- ---------------------------------------------------------------------------
-- Coordination and source-control state
-- ---------------------------------------------------------------------------

CREATE TABLE coordination.intent_statuses (
    code text PRIMARY KEY,
    is_terminal boolean NOT NULL,
    sort_order smallint NOT NULL,
    description text NOT NULL,
    CONSTRAINT intent_statuses_code_ck CHECK (
        code ~ '^[a-z][a-z0-9_]*$'
        AND char_length(code) <= 50
    ),
    CONSTRAINT intent_statuses_sort_order_ck CHECK (sort_order >= 0),
    CONSTRAINT intent_statuses_description_ck CHECK (
        btrim(description) <> ''
        AND char_length(description) <= 500
    ),
    CONSTRAINT intent_statuses_sort_order_uq UNIQUE (sort_order)
);

INSERT INTO coordination.intent_statuses (
    code,
    is_terminal,
    sort_order,
    description
) VALUES
    ('draft', false, 10, 'The intent is being prepared.'),
    ('active', false, 20, 'Work on the intent is active.'),
    ('blocked', false, 30, 'The intent cannot currently advance.'),
    ('submitted', false, 40, 'The intent has been submitted for validation or integration.'),
    ('completed', true, 50, 'The intent finished successfully.'),
    ('cancelled', true, 60, 'The intent was explicitly cancelled.'),
    ('abandoned', true, 70, 'The intent was left without completion.');

CREATE TABLE coordination.intents (
    id uuid PRIMARY KEY DEFAULT uuidv7(),
    organization_id uuid NOT NULL,
    project_id uuid NOT NULL,
    title text NOT NULL,
    goal text NOT NULL,
    success_criteria jsonb NOT NULL DEFAULT '[]'::jsonb,
    status text NOT NULL DEFAULT 'draft',
    base_revision text NOT NULL,
    responsible_agent_id uuid NOT NULL,
    created_by_actor_id uuid NOT NULL,
    version bigint NOT NULL DEFAULT 1,
    created_at timestamptz NOT NULL DEFAULT transaction_timestamp(),
    updated_at timestamptz NOT NULL DEFAULT transaction_timestamp(),
    status_changed_at timestamptz NOT NULL DEFAULT transaction_timestamp(),
    completed_at timestamptz,
    CONSTRAINT intents_project_fk
        FOREIGN KEY (organization_id, project_id)
        REFERENCES identity.projects (organization_id, id)
        ON DELETE RESTRICT,
    CONSTRAINT intents_responsible_agent_fk
        FOREIGN KEY (organization_id, responsible_agent_id)
        REFERENCES identity.agents (organization_id, id)
        ON DELETE RESTRICT,
    CONSTRAINT intents_created_by_actor_fk
        FOREIGN KEY (organization_id, created_by_actor_id)
        REFERENCES identity.actors (organization_id, id)
        ON DELETE RESTRICT,
    CONSTRAINT intents_status_fk FOREIGN KEY (status)
        REFERENCES coordination.intent_statuses (code)
        ON DELETE RESTRICT,
    CONSTRAINT intents_title_ck CHECK (
        btrim(title) <> ''
        AND char_length(title) <= 300
    ),
    CONSTRAINT intents_goal_ck CHECK (
        btrim(goal) <> ''
        AND char_length(goal) <= 10000
    ),
    CONSTRAINT intents_success_criteria_shape_ck CHECK (
        jsonb_typeof(success_criteria) = 'array'
    ),
    CONSTRAINT intents_base_revision_ck CHECK (
        btrim(base_revision) <> ''
        AND char_length(base_revision) <= 255
    ),
    CONSTRAINT intents_version_ck CHECK (version > 0),
    CONSTRAINT intents_completion_state_ck CHECK (
        (status = 'completed' AND completed_at IS NOT NULL)
        OR (status <> 'completed' AND completed_at IS NULL)
    ),
    CONSTRAINT intents_tenant_project_id_uq
        UNIQUE (organization_id, project_id, id)
);

CREATE INDEX intents_project_status_idx
    ON coordination.intents (
        organization_id,
        project_id,
        status,
        updated_at DESC
    );

CREATE INDEX intents_responsible_agent_idx
    ON coordination.intents (
        organization_id,
        responsible_agent_id,
        status,
        updated_at DESC
    );

CREATE TABLE coordination.repositories (
    id uuid PRIMARY KEY DEFAULT uuidv7(),
    organization_id uuid NOT NULL,
    project_id uuid NOT NULL,
    slug text NOT NULL,
    name text NOT NULL,
    vcs_type text NOT NULL DEFAULT 'git',
    status text NOT NULL DEFAULT 'active',
    remote_url text,
    default_branch text NOT NULL DEFAULT 'main',
    object_format text NOT NULL DEFAULT 'sha1',
    settings jsonb NOT NULL DEFAULT '{}'::jsonb,
    version bigint NOT NULL DEFAULT 1,
    created_at timestamptz NOT NULL DEFAULT transaction_timestamp(),
    updated_at timestamptz NOT NULL DEFAULT transaction_timestamp(),
    archived_at timestamptz,
    CONSTRAINT repositories_project_fk
        FOREIGN KEY (organization_id, project_id)
        REFERENCES identity.projects (organization_id, id)
        ON DELETE RESTRICT,
    CONSTRAINT repositories_slug_format_ck CHECK (
        char_length(slug) BETWEEN 1 AND 63
        AND slug ~ '^[a-z0-9][a-z0-9-]*$'
        AND slug !~ '-$'
    ),
    CONSTRAINT repositories_name_ck CHECK (
        btrim(name) <> ''
        AND char_length(name) <= 200
    ),
    CONSTRAINT repositories_vcs_type_ck CHECK (vcs_type = 'git'),
    CONSTRAINT repositories_status_ck CHECK (
        status IN ('active', 'unavailable', 'archived')
    ),
    CONSTRAINT repositories_remote_url_ck CHECK (
        remote_url IS NULL
        OR (
            btrim(remote_url) <> ''
            AND char_length(remote_url) <= 2048
        )
    ),
    CONSTRAINT repositories_default_branch_ck CHECK (
        btrim(default_branch) <> ''
        AND char_length(default_branch) <= 255
    ),
    CONSTRAINT repositories_object_format_ck CHECK (
        object_format IN ('sha1', 'sha256')
    ),
    CONSTRAINT repositories_settings_shape_ck CHECK (
        jsonb_typeof(settings) = 'object'
    ),
    CONSTRAINT repositories_version_ck CHECK (version > 0),
    CONSTRAINT repositories_archive_state_ck CHECK (
        (status = 'archived' AND archived_at IS NOT NULL)
        OR (status <> 'archived' AND archived_at IS NULL)
    ),
    CONSTRAINT repositories_tenant_project_id_uq
        UNIQUE (organization_id, project_id, id),
    CONSTRAINT repositories_tenant_project_slug_uq
        UNIQUE (organization_id, project_id, slug)
);

CREATE INDEX repositories_project_status_idx
    ON coordination.repositories (
        organization_id,
        project_id,
        status,
        updated_at DESC
    );

ALTER TABLE identity.projects
    ADD CONSTRAINT projects_root_repository_fk
    FOREIGN KEY (organization_id, id, root_repository_id)
    REFERENCES coordination.repositories (organization_id, project_id, id)
    ON DELETE RESTRICT;

CREATE TABLE coordination.resource_refs (
    id uuid PRIMARY KEY DEFAULT uuidv7(),
    organization_id uuid NOT NULL,
    project_id uuid NOT NULL,
    repository_id uuid,
    kind text NOT NULL,
    locator text NOT NULL,
    revision text,
    metadata jsonb NOT NULL DEFAULT '{}'::jsonb,
    created_at timestamptz NOT NULL DEFAULT transaction_timestamp(),
    CONSTRAINT resource_refs_project_fk
        FOREIGN KEY (organization_id, project_id)
        REFERENCES identity.projects (organization_id, id)
        ON DELETE RESTRICT,
    CONSTRAINT resource_refs_repository_fk
        FOREIGN KEY (organization_id, project_id, repository_id)
        REFERENCES coordination.repositories (organization_id, project_id, id)
        ON DELETE RESTRICT,
    CONSTRAINT resource_refs_kind_ck CHECK (
        kind IN ('repository', 'path', 'file', 'component', 'symbol')
    ),
    CONSTRAINT resource_refs_repository_presence_ck CHECK (
        kind = 'component' OR repository_id IS NOT NULL
    ),
    CONSTRAINT resource_refs_locator_ck CHECK (
        btrim(locator) <> ''
        AND char_length(locator) <= 4096
    ),
    CONSTRAINT resource_refs_revision_ck CHECK (
        revision IS NULL
        OR (
            btrim(revision) <> ''
            AND char_length(revision) <= 255
        )
    ),
    CONSTRAINT resource_refs_metadata_shape_ck CHECK (
        jsonb_typeof(metadata) = 'object'
    ),
    CONSTRAINT resource_refs_tenant_project_id_uq
        UNIQUE (organization_id, project_id, id),
    CONSTRAINT resource_refs_identity_uq
        UNIQUE NULLS NOT DISTINCT (
            organization_id,
            project_id,
            kind,
            repository_id,
            locator,
            revision
        )
);

CREATE INDEX resource_refs_repository_kind_idx
    ON coordination.resource_refs (
        organization_id,
        project_id,
        repository_id,
        kind
    );

CREATE TABLE coordination.scope_claims (
    id uuid PRIMARY KEY DEFAULT uuidv7(),
    organization_id uuid NOT NULL,
    project_id uuid NOT NULL,
    intent_id uuid NOT NULL,
    session_id uuid,
    resource_ref_id uuid NOT NULL,
    origin text NOT NULL,
    confidence numeric(5,4) NOT NULL DEFAULT 1.0000,
    evidence jsonb NOT NULL DEFAULT '{}'::jsonb,
    status text NOT NULL DEFAULT 'active',
    version bigint NOT NULL DEFAULT 1,
    created_at timestamptz NOT NULL DEFAULT transaction_timestamp(),
    updated_at timestamptz NOT NULL DEFAULT transaction_timestamp(),
    released_at timestamptz,
    CONSTRAINT scope_claims_intent_fk
        FOREIGN KEY (organization_id, project_id, intent_id)
        REFERENCES coordination.intents (organization_id, project_id, id)
        ON DELETE RESTRICT,
    CONSTRAINT scope_claims_session_fk
        FOREIGN KEY (organization_id, project_id, session_id)
        REFERENCES identity.sessions (organization_id, project_id, id)
        ON DELETE RESTRICT,
    CONSTRAINT scope_claims_resource_ref_fk
        FOREIGN KEY (organization_id, project_id, resource_ref_id)
        REFERENCES coordination.resource_refs (organization_id, project_id, id)
        ON DELETE RESTRICT,
    CONSTRAINT scope_claims_origin_ck CHECK (
        origin IN ('declared', 'observed', 'inferred', 'verified')
    ),
    CONSTRAINT scope_claims_confidence_ck CHECK (
        confidence >= 0.0000 AND confidence <= 1.0000
    ),
    CONSTRAINT scope_claims_evidence_shape_ck CHECK (
        jsonb_typeof(evidence) = 'object'
    ),
    CONSTRAINT scope_claims_status_ck CHECK (
        status IN ('active', 'released', 'superseded')
    ),
    CONSTRAINT scope_claims_version_ck CHECK (version > 0),
    CONSTRAINT scope_claims_release_state_ck CHECK (
        (status = 'active' AND released_at IS NULL)
        OR (status IN ('released', 'superseded') AND released_at IS NOT NULL)
    ),
    CONSTRAINT scope_claims_tenant_project_id_uq
        UNIQUE (organization_id, project_id, id)
);

CREATE UNIQUE INDEX scope_claims_one_active_origin_idx
    ON coordination.scope_claims (
        organization_id,
        project_id,
        intent_id,
        resource_ref_id,
        origin
    )
    WHERE status = 'active';

CREATE INDEX scope_claims_active_resource_idx
    ON coordination.scope_claims (
        organization_id,
        project_id,
        resource_ref_id,
        origin
    )
    WHERE status = 'active';

CREATE TABLE coordination.workspaces (
    id uuid PRIMARY KEY DEFAULT uuidv7(),
    organization_id uuid NOT NULL,
    project_id uuid NOT NULL,
    repository_id uuid NOT NULL,
    intent_id uuid NOT NULL,
    session_id uuid NOT NULL,
    base_revision text NOT NULL,
    path_ref text NOT NULL,
    git_branch text NOT NULL,
    status text NOT NULL DEFAULT 'provisioning',
    status_detail jsonb NOT NULL DEFAULT '{}'::jsonb,
    version bigint NOT NULL DEFAULT 1,
    created_at timestamptz NOT NULL DEFAULT transaction_timestamp(),
    updated_at timestamptz NOT NULL DEFAULT transaction_timestamp(),
    frozen_at timestamptz,
    archived_at timestamptz,
    CONSTRAINT workspaces_repository_fk
        FOREIGN KEY (organization_id, project_id, repository_id)
        REFERENCES coordination.repositories (organization_id, project_id, id)
        ON DELETE RESTRICT,
    CONSTRAINT workspaces_intent_fk
        FOREIGN KEY (organization_id, project_id, intent_id)
        REFERENCES coordination.intents (organization_id, project_id, id)
        ON DELETE RESTRICT,
    CONSTRAINT workspaces_session_fk
        FOREIGN KEY (organization_id, project_id, session_id)
        REFERENCES identity.sessions (organization_id, project_id, id)
        ON DELETE RESTRICT,
    CONSTRAINT workspaces_base_revision_ck CHECK (
        btrim(base_revision) <> ''
        AND char_length(base_revision) <= 255
    ),
    CONSTRAINT workspaces_path_ref_ck CHECK (
        btrim(path_ref) <> ''
        AND char_length(path_ref) <= 4096
    ),
    CONSTRAINT workspaces_git_branch_ck CHECK (
        btrim(git_branch) <> ''
        AND char_length(git_branch) <= 255
    ),
    CONSTRAINT workspaces_status_ck CHECK (
        status IN (
            'provisioning',
            'ready',
            'active',
            'frozen',
            'archived',
            'failed'
        )
    ),
    CONSTRAINT workspaces_status_detail_shape_ck CHECK (
        jsonb_typeof(status_detail) = 'object'
    ),
    CONSTRAINT workspaces_version_ck CHECK (version > 0),
    CONSTRAINT workspaces_frozen_state_ck CHECK (
        status <> 'frozen' OR frozen_at IS NOT NULL
    ),
    CONSTRAINT workspaces_archive_state_ck CHECK (
        (status = 'archived' AND archived_at IS NOT NULL)
        OR (status <> 'archived' AND archived_at IS NULL)
    ),
    CONSTRAINT workspaces_tenant_project_id_uq
        UNIQUE (organization_id, project_id, id)
);

CREATE UNIQUE INDEX workspaces_one_live_intent_idx
    ON coordination.workspaces (
        organization_id,
        project_id,
        intent_id
    )
    WHERE status IN ('provisioning', 'ready', 'active', 'frozen');

CREATE INDEX workspaces_session_status_idx
    ON coordination.workspaces (
        organization_id,
        project_id,
        session_id,
        status
    );

CREATE TABLE coordination.changesets (
    id uuid PRIMARY KEY DEFAULT uuidv7(),
    organization_id uuid NOT NULL,
    project_id uuid NOT NULL,
    repository_id uuid NOT NULL,
    workspace_id uuid NOT NULL,
    intent_id uuid NOT NULL,
    base_revision text NOT NULL,
    content_hash bytea NOT NULL,
    git_tree text NOT NULL,
    patch_object_ref text NOT NULL,
    status text NOT NULL DEFAULT 'created',
    metadata jsonb NOT NULL DEFAULT '{}'::jsonb,
    version bigint NOT NULL DEFAULT 1,
    created_at timestamptz NOT NULL DEFAULT transaction_timestamp(),
    updated_at timestamptz NOT NULL DEFAULT transaction_timestamp(),
    CONSTRAINT changesets_repository_fk
        FOREIGN KEY (organization_id, project_id, repository_id)
        REFERENCES coordination.repositories (organization_id, project_id, id)
        ON DELETE RESTRICT,
    CONSTRAINT changesets_workspace_fk
        FOREIGN KEY (organization_id, project_id, workspace_id)
        REFERENCES coordination.workspaces (organization_id, project_id, id)
        ON DELETE RESTRICT,
    CONSTRAINT changesets_intent_fk
        FOREIGN KEY (organization_id, project_id, intent_id)
        REFERENCES coordination.intents (organization_id, project_id, id)
        ON DELETE RESTRICT,
    CONSTRAINT changesets_base_revision_ck CHECK (
        btrim(base_revision) <> ''
        AND char_length(base_revision) <= 255
    ),
    CONSTRAINT changesets_content_hash_ck CHECK (
        octet_length(content_hash) = 32
    ),
    CONSTRAINT changesets_git_tree_ck CHECK (
        btrim(git_tree) <> ''
        AND char_length(git_tree) <= 255
    ),
    CONSTRAINT changesets_patch_object_ref_ck CHECK (
        btrim(patch_object_ref) <> ''
        AND char_length(patch_object_ref) <= 4096
    ),
    CONSTRAINT changesets_status_ck CHECK (
        status IN (
            'created',
            'validating',
            'validated',
            'rejected',
            'integrating',
            'integrated',
            'superseded'
        )
    ),
    CONSTRAINT changesets_metadata_shape_ck CHECK (
        jsonb_typeof(metadata) = 'object'
    ),
    CONSTRAINT changesets_version_ck CHECK (version > 0),
    CONSTRAINT changesets_tenant_project_id_uq
        UNIQUE (organization_id, project_id, id),
    CONSTRAINT changesets_workspace_hash_uq
        UNIQUE (
            organization_id,
            project_id,
            workspace_id,
            content_hash
        )
);

CREATE INDEX changesets_intent_status_idx
    ON coordination.changesets (
        organization_id,
        project_id,
        intent_id,
        status,
        created_at DESC
    );

CREATE OR REPLACE FUNCTION coordination.enforce_changeset_immutability()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
    IF ROW(
        NEW.id,
        NEW.organization_id,
        NEW.project_id,
        NEW.repository_id,
        NEW.workspace_id,
        NEW.intent_id,
        NEW.base_revision,
        NEW.content_hash,
        NEW.git_tree,
        NEW.patch_object_ref,
        NEW.metadata,
        NEW.created_at
    ) IS DISTINCT FROM ROW(
        OLD.id,
        OLD.organization_id,
        OLD.project_id,
        OLD.repository_id,
        OLD.workspace_id,
        OLD.intent_id,
        OLD.base_revision,
        OLD.content_hash,
        OLD.git_tree,
        OLD.patch_object_ref,
        OLD.metadata,
        OLD.created_at
    ) THEN
        RAISE EXCEPTION 'immutable ChangeSet content cannot be updated'
            USING ERRCODE = '23000';
    END IF;

    RETURN NEW;
END;
$$;

CREATE TRIGGER changesets_immutable_content_trg
BEFORE UPDATE ON coordination.changesets
FOR EACH ROW
EXECUTE FUNCTION coordination.enforce_changeset_immutability();

CREATE TABLE coordination.validation_runs (
    id uuid PRIMARY KEY DEFAULT uuidv7(),
    organization_id uuid NOT NULL,
    project_id uuid NOT NULL,
    changeset_id uuid NOT NULL,
    changeset_hash bytea NOT NULL,
    validation_type text NOT NULL,
    status text NOT NULL DEFAULT 'queued',
    result jsonb,
    created_at timestamptz NOT NULL DEFAULT transaction_timestamp(),
    started_at timestamptz,
    finished_at timestamptz,
    CONSTRAINT validation_runs_changeset_fk
        FOREIGN KEY (organization_id, project_id, changeset_id)
        REFERENCES coordination.changesets (organization_id, project_id, id)
        ON DELETE RESTRICT,
    CONSTRAINT validation_runs_changeset_hash_ck CHECK (
        octet_length(changeset_hash) = 32
    ),
    CONSTRAINT validation_runs_type_ck CHECK (
        btrim(validation_type) <> ''
        AND char_length(validation_type) <= 150
    ),
    CONSTRAINT validation_runs_status_ck CHECK (
        status IN (
            'queued',
            'running',
            'passed',
            'failed',
            'cancelled',
            'error'
        )
    ),
    CONSTRAINT validation_runs_result_shape_ck CHECK (
        result IS NULL OR jsonb_typeof(result) = 'object'
    ),
    CONSTRAINT validation_runs_time_state_ck CHECK (
        (
            status = 'queued'
            AND started_at IS NULL
            AND finished_at IS NULL
        )
        OR (
            status = 'running'
            AND started_at IS NOT NULL
            AND finished_at IS NULL
        )
        OR (
            status IN ('passed', 'failed', 'cancelled', 'error')
            AND started_at IS NOT NULL
            AND finished_at IS NOT NULL
            AND finished_at >= started_at
        )
    ),
    CONSTRAINT validation_runs_tenant_project_id_uq
        UNIQUE (organization_id, project_id, id)
);

CREATE INDEX validation_runs_changeset_status_idx
    ON coordination.validation_runs (
        organization_id,
        project_id,
        changeset_id,
        status,
        created_at DESC
    );

CREATE TABLE coordination.integration_attempts (
    id uuid PRIMARY KEY DEFAULT uuidv7(),
    organization_id uuid NOT NULL,
    project_id uuid NOT NULL,
    changeset_id uuid NOT NULL,
    attempt_number integer NOT NULL DEFAULT 1,
    target_revision text NOT NULL,
    status text NOT NULL DEFAULT 'queued',
    result_revision text,
    result jsonb,
    created_at timestamptz NOT NULL DEFAULT transaction_timestamp(),
    started_at timestamptz,
    finished_at timestamptz,
    CONSTRAINT integration_attempts_changeset_fk
        FOREIGN KEY (organization_id, project_id, changeset_id)
        REFERENCES coordination.changesets (organization_id, project_id, id)
        ON DELETE RESTRICT,
    CONSTRAINT integration_attempts_number_ck CHECK (attempt_number > 0),
    CONSTRAINT integration_attempts_target_revision_ck CHECK (
        btrim(target_revision) <> ''
        AND char_length(target_revision) <= 255
    ),
    CONSTRAINT integration_attempts_status_ck CHECK (
        status IN (
            'queued',
            'running',
            'conflicted',
            'succeeded',
            'failed',
            'cancelled'
        )
    ),
    CONSTRAINT integration_attempts_result_revision_ck CHECK (
        result_revision IS NULL
        OR (
            btrim(result_revision) <> ''
            AND char_length(result_revision) <= 255
        )
    ),
    CONSTRAINT integration_attempts_result_shape_ck CHECK (
        result IS NULL OR jsonb_typeof(result) = 'object'
    ),
    CONSTRAINT integration_attempts_success_result_ck CHECK (
        status <> 'succeeded' OR result_revision IS NOT NULL
    ),
    CONSTRAINT integration_attempts_time_state_ck CHECK (
        (
            status = 'queued'
            AND started_at IS NULL
            AND finished_at IS NULL
        )
        OR (
            status = 'running'
            AND started_at IS NOT NULL
            AND finished_at IS NULL
        )
        OR (
            status IN ('conflicted', 'succeeded', 'failed', 'cancelled')
            AND started_at IS NOT NULL
            AND finished_at IS NOT NULL
            AND finished_at >= started_at
        )
    ),
    CONSTRAINT integration_attempts_tenant_project_id_uq
        UNIQUE (organization_id, project_id, id),
    CONSTRAINT integration_attempts_changeset_attempt_uq
        UNIQUE (
            organization_id,
            project_id,
            changeset_id,
            attempt_number
        )
);

CREATE INDEX integration_attempts_changeset_status_idx
    ON coordination.integration_attempts (
        organization_id,
        project_id,
        changeset_id,
        status,
        created_at DESC
    );

-- ---------------------------------------------------------------------------
-- Durable event delivery and command idempotency
-- ---------------------------------------------------------------------------

CREATE TABLE platform.project_event_counters (
    organization_id uuid NOT NULL,
    project_id uuid NOT NULL,
    last_sequence bigint NOT NULL DEFAULT 0,
    updated_at timestamptz NOT NULL DEFAULT transaction_timestamp(),
    CONSTRAINT project_event_counters_pk
        PRIMARY KEY (organization_id, project_id),
    CONSTRAINT project_event_counters_project_fk
        FOREIGN KEY (organization_id, project_id)
        REFERENCES identity.projects (organization_id, id)
        ON DELETE RESTRICT,
    CONSTRAINT project_event_counters_sequence_ck CHECK (last_sequence >= 0)
);

CREATE TABLE platform.events (
    id uuid PRIMARY KEY DEFAULT uuidv7(),
    ingest_sequence bigint GENERATED ALWAYS AS IDENTITY,
    organization_id uuid NOT NULL,
    project_id uuid NOT NULL,
    project_sequence bigint NOT NULL,
    event_type text NOT NULL,
    event_version smallint NOT NULL DEFAULT 1,
    aggregate_type text NOT NULL,
    aggregate_id uuid NOT NULL,
    aggregate_version bigint NOT NULL,
    actor_id uuid,
    session_id uuid,
    intent_id uuid,
    command_id uuid NOT NULL,
    causation_id uuid,
    correlation_id uuid NOT NULL,
    subject text,
    occurred_at timestamptz NOT NULL DEFAULT transaction_timestamp(),
    recorded_at timestamptz NOT NULL DEFAULT transaction_timestamp(),
    git_revision text,
    payload jsonb NOT NULL,
    payload_hash bytea NOT NULL,
    CONSTRAINT events_project_fk
        FOREIGN KEY (organization_id, project_id)
        REFERENCES identity.projects (organization_id, id)
        ON DELETE RESTRICT,
    CONSTRAINT events_actor_fk
        FOREIGN KEY (organization_id, actor_id)
        REFERENCES identity.actors (organization_id, id)
        ON DELETE RESTRICT,
    CONSTRAINT events_session_fk
        FOREIGN KEY (organization_id, project_id, session_id)
        REFERENCES identity.sessions (organization_id, project_id, id)
        ON DELETE RESTRICT,
    CONSTRAINT events_intent_fk
        FOREIGN KEY (organization_id, project_id, intent_id)
        REFERENCES coordination.intents (organization_id, project_id, id)
        ON DELETE RESTRICT,
    CONSTRAINT events_project_sequence_ck CHECK (project_sequence > 0),
    CONSTRAINT events_type_ck CHECK (
        btrim(event_type) <> ''
        AND char_length(event_type) <= 255
    ),
    CONSTRAINT events_event_version_ck CHECK (event_version > 0),
    CONSTRAINT events_aggregate_type_ck CHECK (
        btrim(aggregate_type) <> ''
        AND char_length(aggregate_type) <= 100
    ),
    CONSTRAINT events_aggregate_version_ck CHECK (aggregate_version > 0),
    CONSTRAINT events_subject_ck CHECK (
        subject IS NULL
        OR (
            btrim(subject) <> ''
            AND char_length(subject) <= 500
        )
    ),
    CONSTRAINT events_git_revision_ck CHECK (
        git_revision IS NULL
        OR (
            btrim(git_revision) <> ''
            AND char_length(git_revision) <= 255
        )
    ),
    CONSTRAINT events_payload_shape_ck CHECK (
        jsonb_typeof(payload) = 'object'
    ),
    CONSTRAINT events_payload_hash_ck CHECK (
        octet_length(payload_hash) = 32
    ),
    CONSTRAINT events_ingest_sequence_uq UNIQUE (ingest_sequence),
    CONSTRAINT events_tenant_id_uq UNIQUE (organization_id, id),
    CONSTRAINT events_tenant_project_id_uq
        UNIQUE (organization_id, project_id, id),
    CONSTRAINT events_project_sequence_uq
        UNIQUE (organization_id, project_id, project_sequence),
    CONSTRAINT events_aggregate_version_uq
        UNIQUE (
            organization_id,
            project_id,
            aggregate_type,
            aggregate_id,
            aggregate_version
        )
);

CREATE INDEX events_project_type_sequence_idx
    ON platform.events (
        organization_id,
        project_id,
        event_type,
        project_sequence DESC
    );

CREATE INDEX events_project_correlation_idx
    ON platform.events (
        organization_id,
        project_id,
        correlation_id,
        project_sequence
    );

CREATE OR REPLACE FUNCTION platform.reject_event_mutation()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
    RAISE EXCEPTION 'events are append-only'
        USING ERRCODE = '23000';
END;
$$;

CREATE TRIGGER events_append_only_trg
BEFORE UPDATE OR DELETE ON platform.events
FOR EACH ROW
EXECUTE FUNCTION platform.reject_event_mutation();

CREATE TABLE platform.outbox (
    id uuid PRIMARY KEY DEFAULT uuidv7(),
    organization_id uuid NOT NULL,
    project_id uuid NOT NULL,
    event_id uuid NOT NULL,
    channel text NOT NULL,
    status text NOT NULL DEFAULT 'pending',
    attempts integer NOT NULL DEFAULT 0,
    available_at timestamptz NOT NULL DEFAULT transaction_timestamp(),
    locked_by text,
    locked_until timestamptz,
    published_at timestamptz,
    dead_lettered_at timestamptz,
    last_error text,
    created_at timestamptz NOT NULL DEFAULT transaction_timestamp(),
    updated_at timestamptz NOT NULL DEFAULT transaction_timestamp(),
    CONSTRAINT outbox_event_fk
        FOREIGN KEY (organization_id, project_id, event_id)
        REFERENCES platform.events (organization_id, project_id, id)
        ON DELETE RESTRICT,
    CONSTRAINT outbox_channel_ck CHECK (
        btrim(channel) <> ''
        AND char_length(channel) <= 128
    ),
    CONSTRAINT outbox_status_ck CHECK (
        status IN ('pending', 'published', 'dead_letter')
    ),
    CONSTRAINT outbox_attempts_ck CHECK (attempts >= 0),
    CONSTRAINT outbox_lock_pair_ck CHECK (
        (locked_by IS NULL AND locked_until IS NULL)
        OR (
            locked_by IS NOT NULL
            AND btrim(locked_by) <> ''
            AND locked_until IS NOT NULL
        )
    ),
    CONSTRAINT outbox_terminal_state_ck CHECK (
        (
            status = 'pending'
            AND published_at IS NULL
            AND dead_lettered_at IS NULL
        )
        OR (
            status = 'published'
            AND published_at IS NOT NULL
            AND dead_lettered_at IS NULL
            AND locked_by IS NULL
            AND locked_until IS NULL
        )
        OR (
            status = 'dead_letter'
            AND published_at IS NULL
            AND dead_lettered_at IS NOT NULL
            AND locked_by IS NULL
            AND locked_until IS NULL
        )
    ),
    CONSTRAINT outbox_last_error_ck CHECK (
        last_error IS NULL OR char_length(last_error) <= 8192
    ),
    CONSTRAINT outbox_event_channel_uq
        UNIQUE (organization_id, project_id, event_id, channel)
);

CREATE INDEX outbox_pending_delivery_idx
    ON platform.outbox (available_at, locked_until, created_at)
    WHERE status = 'pending';

CREATE INDEX outbox_project_dead_letter_idx
    ON platform.outbox (
        organization_id,
        project_id,
        dead_lettered_at DESC
    )
    WHERE status = 'dead_letter';

CREATE TABLE platform.idempotency_records (
    id uuid PRIMARY KEY DEFAULT uuidv7(),
    organization_id uuid NOT NULL,
    project_id uuid,
    command_id uuid NOT NULL DEFAULT uuidv7(),
    command_type text NOT NULL,
    idempotency_key text NOT NULL,
    request_hash bytea NOT NULL,
    expected_version bigint,
    outcome text,
    response_status integer,
    response_body jsonb,
    event_id uuid,
    aggregate_id uuid,
    created_at timestamptz NOT NULL DEFAULT transaction_timestamp(),
    completed_at timestamptz,
    expires_at timestamptz,
    CONSTRAINT idempotency_records_organization_fk
        FOREIGN KEY (organization_id)
        REFERENCES identity.organizations (id)
        ON DELETE RESTRICT,
    CONSTRAINT idempotency_records_project_fk
        FOREIGN KEY (organization_id, project_id)
        REFERENCES identity.projects (organization_id, id)
        ON DELETE RESTRICT,
    CONSTRAINT idempotency_records_event_fk
        FOREIGN KEY (organization_id, event_id)
        REFERENCES platform.events (organization_id, id)
        ON DELETE RESTRICT,
    CONSTRAINT idempotency_records_command_type_ck CHECK (
        btrim(command_type) <> ''
        AND char_length(command_type) <= 150
    ),
    CONSTRAINT idempotency_records_key_ck CHECK (
        btrim(idempotency_key) <> ''
        AND char_length(idempotency_key) <= 512
    ),
    CONSTRAINT idempotency_records_request_hash_ck CHECK (
        octet_length(request_hash) = 32
    ),
    CONSTRAINT idempotency_records_expected_version_ck CHECK (
        expected_version IS NULL OR expected_version >= 0
    ),
    CONSTRAINT idempotency_records_outcome_ck CHECK (
        outcome IS NULL OR outcome IN ('succeeded', 'rejected')
    ),
    CONSTRAINT idempotency_records_response_status_ck CHECK (
        response_status IS NULL
        OR response_status BETWEEN 100 AND 599
    ),
    CONSTRAINT idempotency_records_response_shape_ck CHECK (
        response_body IS NULL OR jsonb_typeof(response_body) = 'object'
    ),
    CONSTRAINT idempotency_records_completion_state_ck CHECK (
        (
            outcome IS NULL
            AND response_status IS NULL
            AND completed_at IS NULL
        )
        OR (
            outcome IS NOT NULL
            AND response_status IS NOT NULL
            AND completed_at IS NOT NULL
        )
    ),
    CONSTRAINT idempotency_records_expiry_ck CHECK (
        expires_at IS NULL OR expires_at > created_at
    ),
    CONSTRAINT idempotency_records_command_id_uq UNIQUE (command_id),
    CONSTRAINT idempotency_records_scope_key_uq
        UNIQUE NULLS NOT DISTINCT (
            organization_id,
            project_id,
            command_type,
            idempotency_key
        )
);

CREATE INDEX idempotency_records_expiry_idx
    ON platform.idempotency_records (expires_at)
    WHERE expires_at IS NOT NULL;
