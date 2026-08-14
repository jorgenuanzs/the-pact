-- Reserve "workspace" for Pact's durable collaboration and knowledge boundary.
-- The existing coordination table represents Git worktrees created for an
-- intent. Its public v0.7 API remains compatible while clients migrate.
ALTER TABLE coordination.workspaces RENAME TO worktrees;

ALTER TABLE coordination.worktrees
    RENAME CONSTRAINT workspaces_repository_fk TO worktrees_repository_fk;
ALTER TABLE coordination.worktrees
    RENAME CONSTRAINT workspaces_intent_fk TO worktrees_intent_fk;
ALTER TABLE coordination.worktrees
    RENAME CONSTRAINT workspaces_session_fk TO worktrees_session_fk;
ALTER TABLE coordination.worktrees
    RENAME CONSTRAINT workspaces_base_revision_ck TO worktrees_base_revision_ck;
ALTER TABLE coordination.worktrees
    RENAME CONSTRAINT workspaces_path_ref_ck TO worktrees_path_ref_ck;
ALTER TABLE coordination.worktrees
    RENAME CONSTRAINT workspaces_git_branch_ck TO worktrees_git_branch_ck;
ALTER TABLE coordination.worktrees
    RENAME CONSTRAINT workspaces_status_ck TO worktrees_status_ck;
ALTER TABLE coordination.worktrees
    RENAME CONSTRAINT workspaces_status_detail_shape_ck TO worktrees_status_detail_shape_ck;
ALTER TABLE coordination.worktrees
    RENAME CONSTRAINT workspaces_version_ck TO worktrees_version_ck;
ALTER TABLE coordination.worktrees
    RENAME CONSTRAINT workspaces_frozen_state_ck TO worktrees_frozen_state_ck;
ALTER TABLE coordination.worktrees
    RENAME CONSTRAINT workspaces_archive_state_ck TO worktrees_archive_state_ck;
ALTER TABLE coordination.worktrees
    RENAME CONSTRAINT workspaces_tenant_project_id_uq TO worktrees_tenant_project_id_uq;

ALTER INDEX coordination.workspaces_one_live_intent_idx
    RENAME TO worktrees_one_live_intent_idx;
ALTER INDEX coordination.workspaces_session_status_idx
    RENAME TO worktrees_session_status_idx;

ALTER TABLE coordination.changesets
    RENAME CONSTRAINT changesets_workspace_fk TO changesets_worktree_fk;
ALTER TABLE coordination.changesets
    RENAME CONSTRAINT changesets_workspace_hash_uq TO changesets_worktree_hash_uq;
ALTER TABLE coordination.repository_observations
    RENAME CONSTRAINT repository_observations_workspace_fk TO repository_observations_worktree_fk;

CREATE TABLE identity.workspaces (
    id uuid PRIMARY KEY DEFAULT uuidv7(),
    organization_id uuid NOT NULL,
    slug text NOT NULL,
    name text NOT NULL,
    description text NOT NULL DEFAULT '',
    status text NOT NULL DEFAULT 'active',
    settings jsonb NOT NULL DEFAULT '{}'::jsonb,
    version bigint NOT NULL DEFAULT 1,
    created_at timestamptz NOT NULL DEFAULT transaction_timestamp(),
    updated_at timestamptz NOT NULL DEFAULT transaction_timestamp(),
    archived_at timestamptz,
    CONSTRAINT workspaces_organization_fk FOREIGN KEY (organization_id)
        REFERENCES identity.organizations (id) ON DELETE RESTRICT,
    CONSTRAINT workspaces_slug_format_ck CHECK (
        char_length(slug) BETWEEN 1 AND 63
        AND slug ~ '^[a-z0-9][a-z0-9-]*$'
        AND slug !~ '-$'
    ),
    CONSTRAINT workspaces_name_ck CHECK (
        btrim(name) <> '' AND char_length(name) <= 120
    ),
    CONSTRAINT workspaces_description_ck CHECK (char_length(description) <= 4000),
    CONSTRAINT workspaces_status_ck CHECK (status IN ('active', 'archived')),
    CONSTRAINT workspaces_settings_shape_ck CHECK (jsonb_typeof(settings) = 'object'),
    CONSTRAINT workspaces_version_ck CHECK (version > 0),
    CONSTRAINT workspaces_archive_state_ck CHECK (
        (status = 'archived' AND archived_at IS NOT NULL)
        OR (status <> 'archived' AND archived_at IS NULL)
    ),
    CONSTRAINT workspaces_tenant_id_uq UNIQUE (organization_id, id),
    CONSTRAINT workspaces_tenant_slug_uq UNIQUE (organization_id, slug)
);

CREATE INDEX workspaces_tenant_status_idx
    ON identity.workspaces (organization_id, status, updated_at DESC);

CREATE TABLE identity.workspace_projects (
    organization_id uuid NOT NULL,
    workspace_id uuid NOT NULL,
    project_id uuid NOT NULL,
    added_at timestamptz NOT NULL DEFAULT transaction_timestamp(),
    CONSTRAINT workspace_projects_pk
        PRIMARY KEY (organization_id, workspace_id, project_id),
    CONSTRAINT workspace_projects_workspace_fk
        FOREIGN KEY (organization_id, workspace_id)
        REFERENCES identity.workspaces (organization_id, id) ON DELETE CASCADE,
    CONSTRAINT workspace_projects_project_fk
        FOREIGN KEY (organization_id, project_id)
        REFERENCES identity.projects (organization_id, id) ON DELETE RESTRICT,
    CONSTRAINT workspace_projects_one_workspace_uq
        UNIQUE (organization_id, project_id)
);

CREATE INDEX workspace_projects_workspace_idx
    ON identity.workspace_projects (organization_id, workspace_id, added_at);

CREATE TABLE identity.workspace_members (
    organization_id uuid NOT NULL,
    workspace_id uuid NOT NULL,
    principal_id uuid NOT NULL,
    role text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT transaction_timestamp(),
    updated_at timestamptz NOT NULL DEFAULT transaction_timestamp(),
    CONSTRAINT workspace_members_pk
        PRIMARY KEY (organization_id, workspace_id, principal_id),
    CONSTRAINT workspace_members_workspace_fk
        FOREIGN KEY (organization_id, workspace_id)
        REFERENCES identity.workspaces (organization_id, id) ON DELETE CASCADE,
    CONSTRAINT workspace_members_principal_fk
        FOREIGN KEY (organization_id, principal_id)
        REFERENCES identity.principals (organization_id, id) ON DELETE RESTRICT,
    CONSTRAINT workspace_members_role_ck CHECK (
        role IN ('owner', 'maintainer', 'contributor', 'viewer')
    )
);

CREATE INDEX workspace_members_principal_idx
    ON identity.workspace_members (organization_id, principal_id, workspace_id);

-- Every existing project starts in a one-to-one default workspace. Teams can
-- later move related projects into a shared workspace without changing Git.
INSERT INTO identity.workspaces (
    organization_id, slug, name, description, status, settings, archived_at
)
SELECT organization_id, slug, name, '',
       CASE WHEN status = 'archived' THEN 'archived' ELSE 'active' END,
       '{"managed_default": true}'::jsonb,
       CASE WHEN status = 'archived' THEN archived_at ELSE NULL END
FROM identity.projects;

INSERT INTO identity.workspace_projects (organization_id, workspace_id, project_id)
SELECT project.organization_id, workspace.id, project.id
FROM identity.projects AS project
JOIN identity.workspaces AS workspace
  ON workspace.organization_id = project.organization_id
 AND workspace.slug = project.slug;
