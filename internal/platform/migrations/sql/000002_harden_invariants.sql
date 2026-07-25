ALTER TABLE identity.projects
    ADD CONSTRAINT projects_canonical_revision_format_ck CHECK (
        canonical_revision IS NULL
        OR canonical_revision ~ '^[0-9a-f]{7,64}$'
    );

ALTER TABLE platform.events
    ADD CONSTRAINT events_type_protocol_safe_ck CHECK (
        event_type ~ '^[A-Za-z0-9][A-Za-z0-9._-]*$'
    );

CREATE OR REPLACE FUNCTION platform.set_event_payload_hash()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
    NEW.payload_hash :=
        sha256(convert_to((NEW.payload::jsonb)::text, 'UTF8'));
    RETURN NEW;
END;
$$;

DROP TRIGGER events_append_only_trg ON platform.events;

UPDATE platform.events
SET payload_hash = sha256(convert_to(payload::text, 'UTF8'));

CREATE TRIGGER events_append_only_trg
BEFORE UPDATE OR DELETE ON platform.events
FOR EACH ROW
EXECUTE FUNCTION platform.reject_event_mutation();

CREATE TRIGGER events_set_payload_hash_trg
BEFORE INSERT ON platform.events
FOR EACH ROW
EXECUTE FUNCTION platform.set_event_payload_hash();

ALTER TABLE platform.events
    ADD CONSTRAINT events_payload_hash_matches_ck CHECK (
        payload_hash = sha256(convert_to(payload::text, 'UTF8'))
    );

CREATE OR REPLACE FUNCTION coordination.reject_changeset_delete()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
    RAISE EXCEPTION 'ChangeSets are immutable and cannot be deleted'
        USING ERRCODE = '23000';
END;
$$;

CREATE TRIGGER changesets_reject_delete_trg
BEFORE DELETE ON coordination.changesets
FOR EACH ROW
EXECUTE FUNCTION coordination.reject_changeset_delete();
