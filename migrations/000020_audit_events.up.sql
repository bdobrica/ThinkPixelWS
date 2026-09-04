CREATE TABLE thinkpixelws.audit_events (
    tenant_id uuid NOT NULL,
    workspace_id uuid,
    audit_event_id uuid NOT NULL,
    transaction_id uuid NOT NULL,
    actor_principal text NOT NULL,
    action text NOT NULL,
    target_kind text NOT NULL,
    target_id text NOT NULL,
    decision text NOT NULL,
    outcome text NOT NULL,
    trace_id text,
    request_id text,
    metadata_schema text NOT NULL,
    metadata jsonb NOT NULL,
    occurred_at timestamptz NOT NULL,

    PRIMARY KEY (tenant_id, audit_event_id),
    CONSTRAINT audit_events_workspace_fk
        FOREIGN KEY (tenant_id, workspace_id)
        REFERENCES thinkpixelws.workspaces (tenant_id, workspace_id),
    CONSTRAINT audit_events_event_id_uuidv7 CHECK (
        (get_byte(uuid_send(audit_event_id), 6) >> 4) = 7
    ),
    CONSTRAINT audit_events_transaction_id_uuidv7 CHECK (
        (get_byte(uuid_send(transaction_id), 6) >> 4) = 7
    ),
    CONSTRAINT audit_events_actor CHECK (
        actor_principal = btrim(actor_principal) AND actor_principal <> ''
        AND char_length(actor_principal) <= 256 AND actor_principal !~ '[[:cntrl:]]'
    ),
    CONSTRAINT audit_events_action CHECK (
        char_length(action) <= 128 AND action ~ '^[a-z][a-z0-9]*([.-][a-z0-9]+)*$'
    ),
    CONSTRAINT audit_events_target_kind CHECK (
        char_length(target_kind) <= 128 AND target_kind ~ '^[a-z][a-z0-9]*([.-][a-z0-9]+)*$'
    ),
    CONSTRAINT audit_events_target_id CHECK (
        target_id = btrim(target_id) AND target_id <> ''
        AND char_length(target_id) <= 256 AND target_id !~ '[[:cntrl:]]'
    ),
    CONSTRAINT audit_events_decision CHECK (
        char_length(decision) <= 128 AND decision ~ '^[a-z][a-z0-9]*([.-][a-z0-9]+)*$'
    ),
    CONSTRAINT audit_events_outcome CHECK (
        char_length(outcome) <= 128 AND outcome ~ '^[a-z][a-z0-9]*([.-][a-z0-9]+)*$'
    ),
    CONSTRAINT audit_events_trace_id CHECK (
        trace_id IS NULL OR (
            trace_id = btrim(trace_id) AND trace_id <> ''
            AND char_length(trace_id) <= 128 AND trace_id !~ '[[:cntrl:]]'
        )
    ),
    CONSTRAINT audit_events_request_id CHECK (
        request_id IS NULL OR (
            request_id = btrim(request_id) AND request_id <> ''
            AND char_length(request_id) <= 128 AND request_id !~ '[[:cntrl:]]'
        )
    ),
    CONSTRAINT audit_events_metadata_schema CHECK (
        metadata_schema = btrim(metadata_schema) AND metadata_schema <> ''
        AND char_length(metadata_schema) <= 128 AND metadata_schema !~ '[[:cntrl:]]'
    ),
    CONSTRAINT audit_events_metadata_object CHECK (
        jsonb_typeof(metadata) = 'object' AND octet_length(metadata::text) <= 65536
    )
);

CREATE INDEX audit_events_transaction_idx
    ON thinkpixelws.audit_events (tenant_id, transaction_id, occurred_at, audit_event_id);

CREATE INDEX audit_events_workspace_occurred_idx
    ON thinkpixelws.audit_events (tenant_id, workspace_id, occurred_at, audit_event_id)
    WHERE workspace_id IS NOT NULL;

CREATE FUNCTION thinkpixelws.reject_audit_event_mutation()
RETURNS trigger
LANGUAGE plpgsql
AS $function$
BEGIN
    RAISE EXCEPTION USING ERRCODE = '55000', MESSAGE = 'Audit events are append-only';
END;
$function$;

CREATE TRIGGER audit_events_append_only
BEFORE UPDATE OR DELETE ON thinkpixelws.audit_events
FOR EACH ROW EXECUTE FUNCTION thinkpixelws.reject_audit_event_mutation();

COMMENT ON TABLE thinkpixelws.audit_events IS
    'Append-only tenant-scoped audit records coupled to business mutations by transaction_id.';
COMMENT ON COLUMN thinkpixelws.audit_events.transaction_id IS
    'Application transaction identity shared by the business mutation and its audit record.';
COMMENT ON COLUMN thinkpixelws.audit_events.metadata IS
    'Schema-versioned policy-safe metadata only; never content, credentials, profile handles, signed URLs, or authority.';
