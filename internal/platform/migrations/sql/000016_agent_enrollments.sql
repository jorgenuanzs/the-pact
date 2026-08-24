CREATE TABLE identity.agent_enrollments (
    id uuid PRIMARY KEY DEFAULT uuidv7(),
    organization_id uuid NOT NULL,
    workspace_id uuid NOT NULL,
    project_id uuid NOT NULL,
    agent_id uuid NOT NULL,
    sponsor_principal_id uuid NOT NULL,
    agent_type text NOT NULL,
    client_type text NOT NULL,
    status text NOT NULL DEFAULT 'pending',
    created_at timestamptz NOT NULL DEFAULT transaction_timestamp(),
    updated_at timestamptz NOT NULL DEFAULT transaction_timestamp(),
    activated_at timestamptz,
    last_seen_at timestamptz,
    revoked_at timestamptz,
    CONSTRAINT agent_enrollments_workspace_project_fk
        FOREIGN KEY (organization_id, workspace_id, project_id)
        REFERENCES identity.workspace_projects (organization_id, workspace_id, project_id)
        ON DELETE CASCADE,
    CONSTRAINT agent_enrollments_agent_fk
        FOREIGN KEY (organization_id, agent_id)
        REFERENCES identity.agents (organization_id, id)
        ON DELETE RESTRICT,
    CONSTRAINT agent_enrollments_sponsor_fk
        FOREIGN KEY (organization_id, sponsor_principal_id)
        REFERENCES identity.principals (organization_id, id)
        ON DELETE RESTRICT,
    CONSTRAINT agent_enrollments_type_ck CHECK (
        btrim(agent_type) <> '' AND char_length(agent_type) <= 100
    ),
    CONSTRAINT agent_enrollments_client_ck CHECK (
        btrim(client_type) <> '' AND char_length(client_type) <= 100
    ),
    CONSTRAINT agent_enrollments_status_ck CHECK (
        status IN ('pending', 'active', 'revoked')
    ),
    CONSTRAINT agent_enrollments_activation_ck CHECK (
        (status = 'pending' AND activated_at IS NULL AND revoked_at IS NULL)
        OR (status = 'active' AND activated_at IS NOT NULL AND revoked_at IS NULL)
        OR (status = 'revoked' AND revoked_at IS NOT NULL)
    )
);

CREATE UNIQUE INDEX agent_enrollments_current_uq
    ON identity.agent_enrollments (
        organization_id,
        project_id,
        sponsor_principal_id,
        agent_type
    )
    WHERE status <> 'revoked';

CREATE INDEX agent_enrollments_workspace_status_idx
    ON identity.agent_enrollments (
        organization_id,
        workspace_id,
        status,
        updated_at DESC
    );

CREATE INDEX agent_enrollments_agent_idx
    ON identity.agent_enrollments (
        organization_id,
        agent_id,
        updated_at DESC
    );
