CREATE TABLE coordination.handoffs (
    id uuid PRIMARY KEY DEFAULT uuidv7(),
    organization_id uuid NOT NULL,
    workspace_id uuid NOT NULL,
    project_id uuid NOT NULL,
    intent_id uuid NOT NULL,
    from_session_id uuid NOT NULL,
    from_actor_id uuid NOT NULL,
    to_session_id uuid,
    to_actor_id uuid,
    status text NOT NULL DEFAULT 'offered',
    summary text NOT NULL,
    completed jsonb NOT NULL DEFAULT '[]'::jsonb,
    remaining_work jsonb NOT NULL DEFAULT '[]'::jsonb,
    blockers jsonb NOT NULL DEFAULT '[]'::jsonb,
    next_steps jsonb NOT NULL DEFAULT '[]'::jsonb,
    validations jsonb NOT NULL DEFAULT '[]'::jsonb,
    version bigint NOT NULL DEFAULT 1,
    offered_at timestamptz NOT NULL DEFAULT transaction_timestamp(),
    responded_at timestamptz,
    expires_at timestamptz NOT NULL,
    created_at timestamptz NOT NULL DEFAULT transaction_timestamp(),
    updated_at timestamptz NOT NULL DEFAULT transaction_timestamp(),
    CONSTRAINT handoffs_workspace_project_fk
        FOREIGN KEY (organization_id, workspace_id, project_id)
        REFERENCES identity.workspace_projects (organization_id, workspace_id, project_id)
        ON DELETE RESTRICT,
    CONSTRAINT handoffs_intent_fk
        FOREIGN KEY (organization_id, project_id, intent_id)
        REFERENCES coordination.intents (organization_id, project_id, id)
        ON DELETE RESTRICT,
    CONSTRAINT handoffs_from_session_fk
        FOREIGN KEY (organization_id, project_id, from_session_id)
        REFERENCES identity.sessions (organization_id, project_id, id)
        ON DELETE RESTRICT,
    CONSTRAINT handoffs_to_session_fk
        FOREIGN KEY (organization_id, project_id, to_session_id)
        REFERENCES identity.sessions (organization_id, project_id, id)
        ON DELETE RESTRICT,
    CONSTRAINT handoffs_from_actor_fk
        FOREIGN KEY (organization_id, from_actor_id)
        REFERENCES identity.actors (organization_id, id) ON DELETE RESTRICT,
    CONSTRAINT handoffs_to_actor_fk
        FOREIGN KEY (organization_id, to_actor_id)
        REFERENCES identity.actors (organization_id, id) ON DELETE RESTRICT,
    CONSTRAINT handoffs_status_ck CHECK (
        status IN ('offered', 'accepted', 'withdrawn', 'expired')
    ),
    CONSTRAINT handoffs_summary_ck CHECK (
        btrim(summary) <> '' AND char_length(summary) <= 10000
    ),
    CONSTRAINT handoffs_completed_shape_ck CHECK (jsonb_typeof(completed) = 'array'),
    CONSTRAINT handoffs_remaining_shape_ck CHECK (jsonb_typeof(remaining_work) = 'array'),
    CONSTRAINT handoffs_blockers_shape_ck CHECK (jsonb_typeof(blockers) = 'array'),
    CONSTRAINT handoffs_next_steps_shape_ck CHECK (jsonb_typeof(next_steps) = 'array'),
    CONSTRAINT handoffs_validations_shape_ck CHECK (jsonb_typeof(validations) = 'array'),
    CONSTRAINT handoffs_version_ck CHECK (version > 0),
    CONSTRAINT handoffs_expiry_ck CHECK (expires_at > offered_at),
    CONSTRAINT handoffs_response_state_ck CHECK (
        (status = 'offered' AND responded_at IS NULL AND to_session_id IS NULL AND to_actor_id IS NULL)
        OR (status = 'accepted' AND responded_at IS NOT NULL AND to_session_id IS NOT NULL AND to_actor_id IS NOT NULL)
        OR (status IN ('withdrawn', 'expired') AND responded_at IS NOT NULL AND to_session_id IS NULL AND to_actor_id IS NULL)
    ),
    CONSTRAINT handoffs_tenant_project_id_uq
        UNIQUE (organization_id, project_id, id)
);

CREATE UNIQUE INDEX handoffs_one_offered_intent_idx
    ON coordination.handoffs (organization_id, project_id, intent_id)
    WHERE status = 'offered';

CREATE INDEX handoffs_project_status_idx
    ON coordination.handoffs (
        organization_id, project_id, status, updated_at DESC, id
    );

CREATE TABLE coordination.handoff_records (
    organization_id uuid NOT NULL,
    workspace_id uuid NOT NULL,
    project_id uuid NOT NULL,
    handoff_id uuid NOT NULL,
    record_id uuid NOT NULL,
    created_at timestamptz NOT NULL DEFAULT transaction_timestamp(),
    CONSTRAINT handoff_records_pk
        PRIMARY KEY (organization_id, project_id, handoff_id, record_id),
    CONSTRAINT handoff_records_handoff_fk
        FOREIGN KEY (organization_id, project_id, handoff_id)
        REFERENCES coordination.handoffs (organization_id, project_id, id)
        ON DELETE CASCADE,
    CONSTRAINT handoff_records_record_fk
        FOREIGN KEY (organization_id, workspace_id, record_id)
        REFERENCES knowledge.records (organization_id, workspace_id, id)
        ON DELETE RESTRICT
);

CREATE INDEX handoff_records_record_idx
    ON coordination.handoff_records (
        organization_id, workspace_id, record_id, handoff_id
    );

CREATE TABLE knowledge.context_packs (
    id uuid PRIMARY KEY DEFAULT uuidv7(),
    organization_id uuid NOT NULL,
    workspace_id uuid NOT NULL,
    project_id uuid NOT NULL,
    intent_id uuid NOT NULL,
    requesting_session_id uuid,
    requested_by_actor_id uuid NOT NULL,
    pack_type text NOT NULL,
    consistency text NOT NULL DEFAULT 'eventual',
    event_cursor bigint NOT NULL,
    git_revision text,
    payload jsonb NOT NULL,
    payload_hash bytea NOT NULL,
    source_fingerprint bytea NOT NULL,
    created_at timestamptz NOT NULL DEFAULT transaction_timestamp(),
    expires_at timestamptz NOT NULL,
    CONSTRAINT context_packs_workspace_project_fk
        FOREIGN KEY (organization_id, workspace_id, project_id)
        REFERENCES identity.workspace_projects (organization_id, workspace_id, project_id)
        ON DELETE RESTRICT,
    CONSTRAINT context_packs_intent_fk
        FOREIGN KEY (organization_id, project_id, intent_id)
        REFERENCES coordination.intents (organization_id, project_id, id)
        ON DELETE RESTRICT,
    CONSTRAINT context_packs_session_fk
        FOREIGN KEY (organization_id, project_id, requesting_session_id)
        REFERENCES identity.sessions (organization_id, project_id, id)
        ON DELETE RESTRICT,
    CONSTRAINT context_packs_actor_fk
        FOREIGN KEY (organization_id, requested_by_actor_id)
        REFERENCES identity.actors (organization_id, id) ON DELETE RESTRICT,
    CONSTRAINT context_packs_type_ck CHECK (
        pack_type IN (
            'implementation', 'handoff', 'review', 'onboarding',
            'meeting', 'incident', 'deployment'
        )
    ),
    CONSTRAINT context_packs_consistency_ck CHECK (
        consistency IN ('eventual')
    ),
    CONSTRAINT context_packs_event_cursor_ck CHECK (event_cursor >= 0),
    CONSTRAINT context_packs_git_revision_ck CHECK (
        git_revision IS NULL OR (
            btrim(git_revision) <> '' AND char_length(git_revision) <= 255
        )
    ),
    CONSTRAINT context_packs_payload_shape_ck CHECK (jsonb_typeof(payload) = 'object'),
    CONSTRAINT context_packs_payload_hash_ck CHECK (octet_length(payload_hash) = 32),
    CONSTRAINT context_packs_payload_hash_matches_ck CHECK (
        payload_hash = sha256(convert_to(payload::text, 'UTF8'))
    ),
    CONSTRAINT context_packs_source_fingerprint_ck CHECK (octet_length(source_fingerprint) = 32),
    CONSTRAINT context_packs_source_fingerprint_matches_ck CHECK (
        encode(source_fingerprint, 'hex') = payload #>> '{snapshot,source_fingerprint}'
    ),
    CONSTRAINT context_packs_expiry_ck CHECK (expires_at > created_at),
    CONSTRAINT context_packs_tenant_project_id_uq
        UNIQUE (organization_id, project_id, id)
);

CREATE INDEX context_packs_intent_created_idx
    ON knowledge.context_packs (
        organization_id, project_id, intent_id, created_at DESC, id
    );

CREATE INDEX context_packs_expiry_idx
    ON knowledge.context_packs (expires_at);

CREATE FUNCTION knowledge.prevent_context_pack_update()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
    RAISE EXCEPTION 'Context Packs are immutable and cannot be updated'
        USING ERRCODE = '55000';
END;
$$;

CREATE TRIGGER context_packs_immutable_trg
BEFORE UPDATE ON knowledge.context_packs
FOR EACH ROW
EXECUTE FUNCTION knowledge.prevent_context_pack_update();
