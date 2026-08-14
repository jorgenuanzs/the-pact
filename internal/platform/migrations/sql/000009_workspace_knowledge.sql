CREATE SCHEMA IF NOT EXISTS knowledge;

CREATE TABLE knowledge.resources (
    id uuid PRIMARY KEY DEFAULT uuidv7(),
    organization_id uuid NOT NULL,
    workspace_id uuid NOT NULL,
    kind text NOT NULL,
    title text NOT NULL,
    locator text NOT NULL,
    description text NOT NULL DEFAULT '',
    classification text NOT NULL DEFAULT 'internal',
    status text NOT NULL DEFAULT 'active',
    metadata jsonb NOT NULL DEFAULT '{}'::jsonb,
    created_by_actor_id uuid NOT NULL,
    version bigint NOT NULL DEFAULT 1,
    created_at timestamptz NOT NULL DEFAULT transaction_timestamp(),
    updated_at timestamptz NOT NULL DEFAULT transaction_timestamp(),
    archived_at timestamptz,
    search_document tsvector GENERATED ALWAYS AS (
        setweight(to_tsvector('simple', coalesce(title, '')), 'A') ||
        setweight(to_tsvector('simple', coalesce(description, '')), 'B') ||
        setweight(to_tsvector('simple', coalesce(locator, '')), 'C')
    ) STORED,
    CONSTRAINT resources_workspace_fk
        FOREIGN KEY (organization_id, workspace_id)
        REFERENCES identity.workspaces (organization_id, id) ON DELETE CASCADE,
    CONSTRAINT resources_created_by_actor_fk
        FOREIGN KEY (organization_id, created_by_actor_id)
        REFERENCES identity.actors (organization_id, id) ON DELETE RESTRICT,
    CONSTRAINT resources_kind_ck CHECK (
        kind IN (
            'url', 'repository', 'document', 'pull_request', 'ticket',
            'meeting', 'dashboard', 'infrastructure', 'other'
        )
    ),
    CONSTRAINT resources_title_ck CHECK (
        btrim(title) <> '' AND char_length(title) <= 300
    ),
    CONSTRAINT resources_locator_ck CHECK (
        btrim(locator) <> '' AND char_length(locator) <= 4096
    ),
    CONSTRAINT resources_description_ck CHECK (char_length(description) <= 10000),
    CONSTRAINT resources_classification_ck CHECK (
        classification IN ('public', 'internal', 'confidential', 'restricted')
    ),
    CONSTRAINT resources_status_ck CHECK (status IN ('active', 'archived')),
    CONSTRAINT resources_metadata_shape_ck CHECK (jsonb_typeof(metadata) = 'object'),
    CONSTRAINT resources_version_ck CHECK (version > 0),
    CONSTRAINT resources_archive_state_ck CHECK (
        (status = 'archived' AND archived_at IS NOT NULL)
        OR (status <> 'archived' AND archived_at IS NULL)
    ),
    CONSTRAINT resources_tenant_workspace_id_uq
        UNIQUE (organization_id, workspace_id, id),
    CONSTRAINT resources_workspace_locator_uq
        UNIQUE (organization_id, workspace_id, kind, locator)
);

CREATE INDEX resources_workspace_status_idx
    ON knowledge.resources (
        organization_id, workspace_id, status, updated_at DESC, id
    );

CREATE INDEX resources_search_idx
    ON knowledge.resources USING gin (search_document);

CREATE TABLE knowledge.records (
    id uuid PRIMARY KEY DEFAULT uuidv7(),
    organization_id uuid NOT NULL,
    workspace_id uuid NOT NULL,
    record_type text NOT NULL,
    title text NOT NULL,
    body text NOT NULL,
    status text NOT NULL DEFAULT 'proposed',
    authority text NOT NULL DEFAULT 'team',
    valid_from timestamptz NOT NULL DEFAULT transaction_timestamp(),
    valid_to timestamptz,
    superseded_by_record_id uuid,
    metadata jsonb NOT NULL DEFAULT '{}'::jsonb,
    created_by_actor_id uuid NOT NULL,
    last_changed_by_actor_id uuid NOT NULL,
    version bigint NOT NULL DEFAULT 1,
    created_at timestamptz NOT NULL DEFAULT transaction_timestamp(),
    updated_at timestamptz NOT NULL DEFAULT transaction_timestamp(),
    search_document tsvector GENERATED ALWAYS AS (
        setweight(to_tsvector('simple', coalesce(title, '')), 'A') ||
        setweight(to_tsvector('simple', coalesce(body, '')), 'B') ||
        setweight(to_tsvector('simple', coalesce(record_type, '')), 'C')
    ) STORED,
    CONSTRAINT records_workspace_fk
        FOREIGN KEY (organization_id, workspace_id)
        REFERENCES identity.workspaces (organization_id, id) ON DELETE CASCADE,
    CONSTRAINT records_created_by_actor_fk
        FOREIGN KEY (organization_id, created_by_actor_id)
        REFERENCES identity.actors (organization_id, id) ON DELETE RESTRICT,
    CONSTRAINT records_changed_by_actor_fk
        FOREIGN KEY (organization_id, last_changed_by_actor_id)
        REFERENCES identity.actors (organization_id, id) ON DELETE RESTRICT,
    CONSTRAINT records_type_ck CHECK (
        record_type IN (
            'decision', 'requirement', 'constraint', 'assumption', 'risk',
            'open_question', 'finding', 'procedure', 'incident',
            'validation_result', 'note'
        )
    ),
    CONSTRAINT records_title_ck CHECK (
        btrim(title) <> '' AND char_length(title) <= 300
    ),
    CONSTRAINT records_body_ck CHECK (
        btrim(body) <> '' AND char_length(body) <= 50000
    ),
    CONSTRAINT records_status_ck CHECK (
        status IN (
            'proposed', 'accepted', 'disputed', 'superseded',
            'revoked', 'expired', 'rejected'
        )
    ),
    CONSTRAINT records_authority_ck CHECK (
        authority IN ('informational', 'team', 'organization', 'external')
    ),
    CONSTRAINT records_validity_ck CHECK (
        valid_to IS NULL OR valid_to >= valid_from
    ),
    CONSTRAINT records_metadata_shape_ck CHECK (jsonb_typeof(metadata) = 'object'),
    CONSTRAINT records_version_ck CHECK (version > 0),
    CONSTRAINT records_terminal_validity_ck CHECK (
        status NOT IN ('superseded', 'revoked', 'expired') OR valid_to IS NOT NULL
    ),
    CONSTRAINT records_superseded_reference_ck CHECK (
        (status = 'superseded' AND superseded_by_record_id IS NOT NULL)
        OR (status <> 'superseded' AND superseded_by_record_id IS NULL)
    ),
    CONSTRAINT records_tenant_workspace_id_uq
        UNIQUE (organization_id, workspace_id, id),
    CONSTRAINT records_superseded_by_fk
        FOREIGN KEY (organization_id, workspace_id, superseded_by_record_id)
        REFERENCES knowledge.records (organization_id, workspace_id, id)
        ON DELETE RESTRICT
);

CREATE INDEX records_workspace_status_idx
    ON knowledge.records (
        organization_id, workspace_id, status, record_type, updated_at DESC, id
    );

CREATE INDEX records_search_idx
    ON knowledge.records USING gin (search_document);

CREATE TABLE knowledge.record_evidence (
    organization_id uuid NOT NULL,
    workspace_id uuid NOT NULL,
    record_id uuid NOT NULL,
    resource_id uuid NOT NULL,
    relation text NOT NULL DEFAULT 'supports',
    note text NOT NULL DEFAULT '',
    created_by_actor_id uuid NOT NULL,
    created_at timestamptz NOT NULL DEFAULT transaction_timestamp(),
    CONSTRAINT record_evidence_pk
        PRIMARY KEY (organization_id, workspace_id, record_id, resource_id, relation),
    CONSTRAINT record_evidence_record_fk
        FOREIGN KEY (organization_id, workspace_id, record_id)
        REFERENCES knowledge.records (organization_id, workspace_id, id)
        ON DELETE CASCADE,
    CONSTRAINT record_evidence_resource_fk
        FOREIGN KEY (organization_id, workspace_id, resource_id)
        REFERENCES knowledge.resources (organization_id, workspace_id, id)
        ON DELETE RESTRICT,
    CONSTRAINT record_evidence_actor_fk
        FOREIGN KEY (organization_id, created_by_actor_id)
        REFERENCES identity.actors (organization_id, id) ON DELETE RESTRICT,
    CONSTRAINT record_evidence_relation_ck CHECK (
        relation IN ('supports', 'contradicts', 'origin', 'validates')
    ),
    CONSTRAINT record_evidence_note_ck CHECK (char_length(note) <= 4000)
);

CREATE INDEX record_evidence_resource_idx
    ON knowledge.record_evidence (
        organization_id, workspace_id, resource_id, record_id
    );

CREATE TABLE knowledge.events (
    id uuid PRIMARY KEY DEFAULT uuidv7(),
    ingest_sequence bigint GENERATED ALWAYS AS IDENTITY,
    organization_id uuid NOT NULL,
    workspace_id uuid NOT NULL,
    event_type text NOT NULL,
    aggregate_type text NOT NULL,
    aggregate_id uuid NOT NULL,
    aggregate_version bigint NOT NULL,
    actor_id uuid NOT NULL,
    payload jsonb NOT NULL,
    payload_hash bytea NOT NULL,
    occurred_at timestamptz NOT NULL DEFAULT transaction_timestamp(),
    CONSTRAINT knowledge_events_workspace_fk
        FOREIGN KEY (organization_id, workspace_id)
        REFERENCES identity.workspaces (organization_id, id) ON DELETE CASCADE,
    CONSTRAINT knowledge_events_actor_fk
        FOREIGN KEY (organization_id, actor_id)
        REFERENCES identity.actors (organization_id, id) ON DELETE RESTRICT,
    CONSTRAINT knowledge_events_type_ck CHECK (
        btrim(event_type) <> '' AND char_length(event_type) <= 255
    ),
    CONSTRAINT knowledge_events_aggregate_type_ck CHECK (
        aggregate_type IN ('resource', 'record')
    ),
    CONSTRAINT knowledge_events_aggregate_version_ck CHECK (aggregate_version > 0),
    CONSTRAINT knowledge_events_payload_shape_ck CHECK (jsonb_typeof(payload) = 'object'),
    CONSTRAINT knowledge_events_payload_hash_ck CHECK (octet_length(payload_hash) = 32),
    CONSTRAINT knowledge_events_ingest_sequence_uq UNIQUE (ingest_sequence),
    CONSTRAINT knowledge_events_tenant_workspace_id_uq
        UNIQUE (organization_id, workspace_id, id)
);

CREATE INDEX knowledge_events_workspace_sequence_idx
    ON knowledge.events (
        organization_id, workspace_id, ingest_sequence DESC
    );
