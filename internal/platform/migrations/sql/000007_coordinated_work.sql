ALTER TABLE coordination.intents
    ADD COLUMN summary text,
    ADD COLUMN status_detail jsonb NOT NULL DEFAULT '{}'::jsonb,
    ADD CONSTRAINT intents_summary_ck CHECK (
        summary IS NULL
        OR (
            btrim(summary) <> ''
            AND char_length(summary) <= 10000
        )
    ),
    ADD CONSTRAINT intents_status_detail_shape_ck CHECK (
        jsonb_typeof(status_detail) = 'object'
    );

ALTER TABLE coordination.scope_claims
    ADD COLUMN claim_mode text NOT NULL DEFAULT 'exclusive',
    ADD COLUMN last_renewed_at timestamptz,
    ADD COLUMN expires_at timestamptz;

UPDATE coordination.scope_claims
SET last_renewed_at = created_at,
    expires_at = created_at + interval '5 minutes';

ALTER TABLE coordination.scope_claims
    ALTER COLUMN last_renewed_at SET NOT NULL,
    ALTER COLUMN expires_at SET NOT NULL,
    ADD CONSTRAINT scope_claims_mode_ck CHECK (
        claim_mode IN ('exclusive', 'shared')
    ),
    ADD CONSTRAINT scope_claims_lease_order_ck CHECK (
        last_renewed_at >= created_at
        AND expires_at > last_renewed_at
    );

CREATE INDEX scope_claims_project_lease_idx
    ON coordination.scope_claims (
        organization_id,
        project_id,
        expires_at,
        updated_at DESC
    )
    WHERE status = 'active';

ALTER TABLE coordination.repository_observations
    ADD COLUMN workspace_id uuid,
    ADD CONSTRAINT repository_observations_workspace_fk
        FOREIGN KEY (organization_id, project_id, workspace_id)
        REFERENCES coordination.workspaces (organization_id, project_id, id)
        ON DELETE RESTRICT;

ALTER TABLE coordination.repository_observations
    DROP CONSTRAINT repository_observations_tenant_session_uq,
    ADD CONSTRAINT repository_observations_tenant_session_workspace_uq
        UNIQUE NULLS NOT DISTINCT (
            organization_id,
            project_id,
            session_id,
            workspace_id
        );

CREATE INDEX repository_observations_workspace_observed_idx
    ON coordination.repository_observations (
        organization_id,
        project_id,
        workspace_id,
        observed_at DESC
    )
    WHERE workspace_id IS NOT NULL;
