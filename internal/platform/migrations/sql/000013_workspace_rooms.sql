CREATE SCHEMA IF NOT EXISTS collaboration;

CREATE TABLE collaboration.rooms (
    id uuid PRIMARY KEY DEFAULT uuidv7(),
    organization_id uuid NOT NULL,
    workspace_id uuid NOT NULL,
    slug text NOT NULL,
    name text NOT NULL,
    description text NOT NULL DEFAULT '',
    status text NOT NULL DEFAULT 'active',
    managed_default boolean NOT NULL DEFAULT false,
    created_by_actor_id uuid,
    version bigint NOT NULL DEFAULT 1,
    created_at timestamptz NOT NULL DEFAULT transaction_timestamp(),
    updated_at timestamptz NOT NULL DEFAULT transaction_timestamp(),
    last_message_at timestamptz,
    archived_at timestamptz,
    CONSTRAINT rooms_workspace_fk
        FOREIGN KEY (organization_id, workspace_id)
        REFERENCES identity.workspaces (organization_id, id) ON DELETE CASCADE,
    CONSTRAINT rooms_created_by_actor_fk
        FOREIGN KEY (organization_id, created_by_actor_id)
        REFERENCES identity.actors (organization_id, id) ON DELETE RESTRICT,
    CONSTRAINT rooms_slug_format_ck CHECK (
        char_length(slug) BETWEEN 1 AND 63
        AND slug ~ '^[a-z0-9][a-z0-9-]*$'
        AND slug !~ '-$'
    ),
    CONSTRAINT rooms_name_ck CHECK (
        btrim(name) <> '' AND char_length(name) <= 120
    ),
    CONSTRAINT rooms_description_ck CHECK (char_length(description) <= 2000),
    CONSTRAINT rooms_status_ck CHECK (status IN ('active', 'archived')),
    CONSTRAINT rooms_version_ck CHECK (version > 0),
    CONSTRAINT rooms_archive_state_ck CHECK (
        (status = 'archived' AND archived_at IS NOT NULL)
        OR (status <> 'archived' AND archived_at IS NULL)
    ),
    CONSTRAINT rooms_tenant_workspace_id_uq
        UNIQUE (organization_id, workspace_id, id),
    CONSTRAINT rooms_workspace_slug_uq
        UNIQUE (organization_id, workspace_id, slug)
);

CREATE INDEX rooms_workspace_activity_idx
    ON collaboration.rooms (
        organization_id, workspace_id, status,
        last_message_at DESC NULLS LAST, created_at, id
    );

CREATE TABLE collaboration.messages (
    id uuid PRIMARY KEY DEFAULT uuidv7(),
    ingest_sequence bigint GENERATED ALWAYS AS IDENTITY,
    organization_id uuid NOT NULL,
    workspace_id uuid NOT NULL,
    room_id uuid NOT NULL,
    author_actor_id uuid NOT NULL,
    author_session_id uuid,
    reply_to_message_id uuid,
    thread_root_message_id uuid,
    body text NOT NULL,
    metadata jsonb NOT NULL DEFAULT '{}'::jsonb,
    created_at timestamptz NOT NULL DEFAULT transaction_timestamp(),
    search_document tsvector GENERATED ALWAYS AS (
        to_tsvector('simple', coalesce(body, ''))
    ) STORED,
    CONSTRAINT messages_room_fk
        FOREIGN KEY (organization_id, workspace_id, room_id)
        REFERENCES collaboration.rooms (organization_id, workspace_id, id)
        ON DELETE CASCADE,
    CONSTRAINT messages_author_actor_fk
        FOREIGN KEY (organization_id, author_actor_id)
        REFERENCES identity.actors (organization_id, id) ON DELETE RESTRICT,
    CONSTRAINT messages_author_session_fk
        FOREIGN KEY (author_session_id)
        REFERENCES identity.sessions (id) ON DELETE SET NULL,
    CONSTRAINT messages_body_ck CHECK (
        btrim(body) <> '' AND char_length(body) <= 50000
    ),
    CONSTRAINT messages_metadata_shape_ck CHECK (jsonb_typeof(metadata) = 'object'),
    CONSTRAINT messages_ingest_sequence_uq UNIQUE (ingest_sequence),
    CONSTRAINT messages_tenant_room_id_uq
        UNIQUE (organization_id, workspace_id, room_id, id),
    CONSTRAINT messages_reply_fk
        FOREIGN KEY (
            organization_id, workspace_id, room_id, reply_to_message_id
        ) REFERENCES collaboration.messages (
            organization_id, workspace_id, room_id, id
        ) ON DELETE RESTRICT,
    CONSTRAINT messages_thread_root_fk
        FOREIGN KEY (
            organization_id, workspace_id, room_id, thread_root_message_id
        ) REFERENCES collaboration.messages (
            organization_id, workspace_id, room_id, id
        ) ON DELETE RESTRICT,
    CONSTRAINT messages_thread_shape_ck CHECK (
        (reply_to_message_id IS NULL AND thread_root_message_id IS NULL)
        OR (reply_to_message_id IS NOT NULL AND thread_root_message_id IS NOT NULL)
    )
);

CREATE INDEX messages_room_sequence_idx
    ON collaboration.messages (
        organization_id, workspace_id, room_id, ingest_sequence DESC
    );

CREATE INDEX messages_thread_sequence_idx
    ON collaboration.messages (
        organization_id, workspace_id, room_id,
        thread_root_message_id, ingest_sequence
    ) WHERE thread_root_message_id IS NOT NULL;

CREATE INDEX messages_search_idx
    ON collaboration.messages USING gin (search_document);

CREATE TABLE collaboration.mentions (
    id uuid PRIMARY KEY DEFAULT uuidv7(),
    organization_id uuid NOT NULL,
    workspace_id uuid NOT NULL,
    room_id uuid NOT NULL,
    message_id uuid NOT NULL,
    mentioned_actor_id uuid NOT NULL,
    status text NOT NULL DEFAULT 'pending',
    created_at timestamptz NOT NULL DEFAULT transaction_timestamp(),
    read_at timestamptz,
    responded_at timestamptz,
    dismissed_at timestamptz,
    CONSTRAINT mentions_message_fk
        FOREIGN KEY (organization_id, workspace_id, room_id, message_id)
        REFERENCES collaboration.messages (
            organization_id, workspace_id, room_id, id
        ) ON DELETE CASCADE,
    CONSTRAINT mentions_actor_fk
        FOREIGN KEY (organization_id, mentioned_actor_id)
        REFERENCES identity.actors (organization_id, id) ON DELETE CASCADE,
    CONSTRAINT mentions_status_ck CHECK (
        status IN ('pending', 'read', 'responded', 'dismissed')
    ),
    CONSTRAINT mentions_state_ck CHECK (
        (status = 'pending' AND read_at IS NULL AND responded_at IS NULL AND dismissed_at IS NULL)
        OR (status = 'read' AND read_at IS NOT NULL AND responded_at IS NULL AND dismissed_at IS NULL)
        OR (status = 'responded' AND read_at IS NOT NULL AND responded_at IS NOT NULL AND dismissed_at IS NULL)
        OR (status = 'dismissed' AND responded_at IS NULL AND dismissed_at IS NOT NULL)
    ),
    CONSTRAINT mentions_tenant_id_uq UNIQUE (organization_id, id),
    CONSTRAINT mentions_message_actor_uq
        UNIQUE (organization_id, message_id, mentioned_actor_id)
);

CREATE INDEX mentions_actor_inbox_idx
    ON collaboration.mentions (
        organization_id, mentioned_actor_id, status, created_at DESC, id
    );

CREATE OR REPLACE FUNCTION collaboration.touch_room_after_message()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
    UPDATE collaboration.rooms
    SET last_message_at = NEW.created_at,
        updated_at = NEW.created_at,
        version = version + 1
    WHERE organization_id = NEW.organization_id
      AND workspace_id = NEW.workspace_id
      AND id = NEW.room_id;
    RETURN NEW;
END;
$$;

CREATE TRIGGER messages_touch_room_trg
AFTER INSERT ON collaboration.messages
FOR EACH ROW
EXECUTE FUNCTION collaboration.touch_room_after_message();

CREATE OR REPLACE FUNCTION collaboration.create_default_room()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
    INSERT INTO collaboration.rooms (
        organization_id, workspace_id, slug, name, description, managed_default
    ) VALUES (
        NEW.organization_id, NEW.id, 'general', 'General',
        'Shared conversation and soft context for this workspace.', true
    ) ON CONFLICT (organization_id, workspace_id, slug) DO NOTHING;
    RETURN NEW;
END;
$$;

CREATE TRIGGER workspaces_create_default_room_trg
AFTER INSERT ON identity.workspaces
FOR EACH ROW
EXECUTE FUNCTION collaboration.create_default_room();

INSERT INTO collaboration.rooms (
    organization_id, workspace_id, slug, name, description, managed_default
)
SELECT organization_id, id, 'general', 'General',
       'Shared conversation and soft context for this workspace.', true
FROM identity.workspaces
ON CONFLICT (organization_id, workspace_id, slug) DO NOTHING;
