-- Organization-wide user administration builds on the invitation and local
-- account model introduced in migration 000014. Invitations may now grant an
-- organization role and optionally seed one project permission.

ALTER TABLE identity.invitations
    ADD COLUMN organization_role text;

UPDATE identity.invitations
SET organization_role = CASE WHEN role = 'owner' THEN 'owner' ELSE 'member' END;

ALTER TABLE identity.invitations
    ALTER COLUMN organization_role SET NOT NULL,
    ALTER COLUMN organization_role SET DEFAULT 'member',
    ALTER COLUMN project_id DROP NOT NULL,
    ALTER COLUMN role DROP NOT NULL;

ALTER TABLE identity.invitations
    DROP CONSTRAINT invitations_role_ck;

ALTER TABLE identity.invitations
    ADD CONSTRAINT invitations_organization_role_ck CHECK (
        organization_role IN ('owner', 'admin', 'member')
    ),
    ADD CONSTRAINT invitations_role_ck CHECK (
        role IS NULL OR role IN ('owner', 'maintainer', 'contributor', 'viewer')
    ),
    ADD CONSTRAINT invitations_project_scope_ck CHECK (
        (project_id IS NULL AND role IS NULL)
        OR (project_id IS NOT NULL AND role IS NOT NULL)
    );

DROP INDEX identity.invitations_pending_email_uq;

-- The previous model allowed the same address to have one pending invitation
-- per project. Keep the newest invitation usable before enforcing the new
-- organization-wide uniqueness rule.
WITH ranked_pending AS (
    SELECT id,
           row_number() OVER (
               PARTITION BY organization_id, lower(email)
               ORDER BY (expires_at > transaction_timestamp()) DESC, created_at DESC, id DESC
           ) AS position
    FROM identity.invitations
    WHERE status = 'pending'
)
UPDATE identity.invitations AS invitation
SET status = 'revoked', revoked_at = transaction_timestamp()
FROM ranked_pending
WHERE ranked_pending.id = invitation.id
  AND ranked_pending.position > 1;

CREATE UNIQUE INDEX invitations_pending_email_uq
    ON identity.invitations (organization_id, lower(email))
    WHERE status = 'pending';

CREATE TABLE identity.user_admin_events (
    id uuid PRIMARY KEY DEFAULT uuidv7(),
    organization_id uuid NOT NULL,
    actor_principal_id uuid NOT NULL,
    target_principal_id uuid,
    action text NOT NULL,
    details jsonb NOT NULL DEFAULT '{}'::jsonb,
    created_at timestamptz NOT NULL DEFAULT transaction_timestamp(),
    CONSTRAINT user_admin_events_organization_fk
        FOREIGN KEY (organization_id)
        REFERENCES identity.organizations (id) ON DELETE RESTRICT,
    CONSTRAINT user_admin_events_actor_fk
        FOREIGN KEY (organization_id, actor_principal_id)
        REFERENCES identity.principals (organization_id, id) ON DELETE RESTRICT,
    CONSTRAINT user_admin_events_target_fk
        FOREIGN KEY (organization_id, target_principal_id)
        REFERENCES identity.principals (organization_id, id) ON DELETE RESTRICT,
    CONSTRAINT user_admin_events_action_ck CHECK (
        btrim(action) <> '' AND char_length(action) <= 100
    ),
    CONSTRAINT user_admin_events_details_shape_ck CHECK (
        jsonb_typeof(details) = 'object'
    ),
    CONSTRAINT user_admin_events_tenant_id_uq UNIQUE (organization_id, id)
);

CREATE INDEX user_admin_events_recent_idx
    ON identity.user_admin_events (organization_id, created_at DESC, id DESC);

CREATE INDEX user_admin_events_target_idx
    ON identity.user_admin_events (organization_id, target_principal_id, created_at DESC)
    WHERE target_principal_id IS NOT NULL;
