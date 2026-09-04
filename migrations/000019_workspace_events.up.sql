CREATE TABLE thinkpixelws.workspace_events (
    tenant_id uuid NOT NULL,
    workspace_id uuid NOT NULL,
    event_id uuid NOT NULL,
    sequence bigint NOT NULL,
    aggregate_version bigint NOT NULL,
    event_type text NOT NULL,
    actor_principal text NOT NULL,
    run_id uuid,
    execution_id uuid,
    trace_id text,
    request_id text,
    payload_schema text NOT NULL,
    payload jsonb NOT NULL,
    occurred_at timestamptz NOT NULL,

    PRIMARY KEY (tenant_id, event_id),
    CONSTRAINT workspace_events_workspace_sequence_unique
        UNIQUE (tenant_id, workspace_id, sequence),
    CONSTRAINT workspace_events_workspace_fk
        FOREIGN KEY (tenant_id, workspace_id)
        REFERENCES thinkpixelws.workspaces (tenant_id, workspace_id),
    CONSTRAINT workspace_events_event_id_uuidv7 CHECK (
        (get_byte(uuid_send(event_id), 6) >> 4) = 7
    ),
    CONSTRAINT workspace_events_sequence_positive CHECK (sequence >= 1),
    CONSTRAINT workspace_events_aggregate_version_positive CHECK (aggregate_version >= 1),
    CONSTRAINT workspace_events_type CHECK (
        char_length(event_type) <= 128
        AND event_type ~ '^workspace\.thinkpixel\.io/[a-z0-9]+(-[a-z0-9]+)*\.[a-z0-9]+(-[a-z0-9]+)*\.v[1-9][0-9]*$'
    ),
    CONSTRAINT workspace_events_actor CHECK (
        actor_principal = btrim(actor_principal)
        AND actor_principal <> ''
        AND char_length(actor_principal) <= 256
        AND actor_principal !~ '[[:cntrl:]]'
    ),
    CONSTRAINT workspace_events_run_id_uuidv7 CHECK (
        run_id IS NULL OR (get_byte(uuid_send(run_id), 6) >> 4) = 7
    ),
    CONSTRAINT workspace_events_execution_id_uuidv7 CHECK (
        execution_id IS NULL OR (get_byte(uuid_send(execution_id), 6) >> 4) = 7
    ),
    CONSTRAINT workspace_events_trace_id CHECK (
        trace_id IS NULL OR (
            trace_id = btrim(trace_id) AND trace_id <> ''
            AND char_length(trace_id) <= 128 AND trace_id !~ '[[:cntrl:]]'
        )
    ),
    CONSTRAINT workspace_events_request_id CHECK (
        request_id IS NULL OR (
            request_id = btrim(request_id) AND request_id <> ''
            AND char_length(request_id) <= 128 AND request_id !~ '[[:cntrl:]]'
        )
    ),
    CONSTRAINT workspace_events_payload_schema CHECK (
        payload_schema = btrim(payload_schema)
        AND payload_schema <> ''
        AND char_length(payload_schema) <= 128
        AND payload_schema !~ '[[:cntrl:]]'
    ),
    CONSTRAINT workspace_events_payload_object CHECK (
        jsonb_typeof(payload) = 'object' AND octet_length(payload::text) <= 65536
    )
);

CREATE INDEX workspace_events_workspace_occurred_idx
    ON thinkpixelws.workspace_events (tenant_id, workspace_id, occurred_at, sequence);

CREATE FUNCTION thinkpixelws.reject_workspace_event_mutation()
RETURNS trigger
LANGUAGE plpgsql
AS $function$
BEGIN
    RAISE EXCEPTION USING ERRCODE = '55000', MESSAGE = 'Workspace events are append-only';
END;
$function$;

CREATE TRIGGER workspace_events_append_only
BEFORE UPDATE OR DELETE ON thinkpixelws.workspace_events
FOR EACH ROW EXECUTE FUNCTION thinkpixelws.reject_workspace_event_mutation();

COMMENT ON TABLE thinkpixelws.workspace_events IS
    'Append-only tenant-scoped Workspace event stream, strictly ordered by sequence within each Workspace.';
COMMENT ON COLUMN thinkpixelws.workspace_events.payload IS
    'Schema-versioned policy-safe metadata only; never Workspace content, credentials, profile handles, signed URLs, or authority.';
