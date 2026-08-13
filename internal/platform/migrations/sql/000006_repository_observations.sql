CREATE TABLE coordination.repository_observations (
    id uuid PRIMARY KEY DEFAULT uuidv7(),
    organization_id uuid NOT NULL,
    project_id uuid NOT NULL,
    session_id uuid NOT NULL,
    actor_id uuid NOT NULL,
    node_id uuid NOT NULL,
    worktree_dirty boolean NOT NULL,
    diff_fingerprint bytea NOT NULL,
    changed_paths integer NOT NULL,
    git_revision text,
    git_branch text,
    version bigint NOT NULL DEFAULT 1,
    created_at timestamptz NOT NULL DEFAULT transaction_timestamp(),
    observed_at timestamptz NOT NULL DEFAULT transaction_timestamp(),
    CONSTRAINT repository_observations_session_fk
        FOREIGN KEY (organization_id, project_id, session_id)
        REFERENCES identity.sessions (organization_id, project_id, id)
        ON DELETE RESTRICT,
    CONSTRAINT repository_observations_actor_fk
        FOREIGN KEY (organization_id, actor_id)
        REFERENCES identity.actors (organization_id, id)
        ON DELETE RESTRICT,
    CONSTRAINT repository_observations_node_fk
        FOREIGN KEY (organization_id, node_id)
        REFERENCES identity.nodes (organization_id, id)
        ON DELETE RESTRICT,
    CONSTRAINT repository_observations_fingerprint_ck CHECK (
        octet_length(diff_fingerprint) = 32
    ),
    CONSTRAINT repository_observations_changed_paths_ck CHECK (
        changed_paths >= 0
        AND (
            (worktree_dirty AND changed_paths > 0)
            OR (NOT worktree_dirty AND changed_paths = 0)
        )
    ),
    CONSTRAINT repository_observations_git_revision_ck CHECK (
        git_revision IS NULL
        OR (
            btrim(git_revision) <> ''
            AND char_length(git_revision) >= 7
            AND char_length(git_revision) <= 64
        )
    ),
    CONSTRAINT repository_observations_git_branch_ck CHECK (
        git_branch IS NULL
        OR (
            btrim(git_branch) <> ''
            AND char_length(git_branch) <= 255
        )
    ),
    CONSTRAINT repository_observations_version_ck CHECK (version > 0),
    CONSTRAINT repository_observations_tenant_session_uq
        UNIQUE (organization_id, project_id, session_id)
);

CREATE INDEX repository_observations_project_observed_idx
    ON coordination.repository_observations (
        organization_id,
        project_id,
        observed_at DESC
    );
