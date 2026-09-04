CREATE TABLE thinkpixelws.outbox_messages (
    tenant_id uuid NOT NULL,
    event_id uuid NOT NULL,
    transaction_id uuid NOT NULL,
    aggregate_kind text NOT NULL,
    aggregate_id uuid NOT NULL,
    aggregate_version bigint NOT NULL,
    sequence bigint NOT NULL,
    event_type text NOT NULL,
    event_version integer NOT NULL,
    payload_schema text NOT NULL,
    payload jsonb NOT NULL,
    attempts integer NOT NULL DEFAULT 0,
    occurred_at timestamptz NOT NULL,
    available_at timestamptz NOT NULL,
    published_at timestamptz,

    PRIMARY KEY (tenant_id, event_id),
    CONSTRAINT outbox_messages_tenant_fk FOREIGN KEY (tenant_id)
        REFERENCES thinkpixelws.tenants (tenant_id),
    CONSTRAINT outbox_messages_event_id_uuidv7 CHECK ((get_byte(uuid_send(event_id), 6) >> 4) = 7),
    CONSTRAINT outbox_messages_transaction_id_uuidv7 CHECK ((get_byte(uuid_send(transaction_id), 6) >> 4) = 7),
    CONSTRAINT outbox_messages_aggregate_id_uuidv7 CHECK ((get_byte(uuid_send(aggregate_id), 6) >> 4) = 7),
    CONSTRAINT outbox_messages_aggregate_kind CHECK (
        char_length(aggregate_kind) <= 128 AND aggregate_kind ~ '^[a-z][a-z0-9]*([.-][a-z0-9]+)*$'
    ),
    CONSTRAINT outbox_messages_aggregate_version CHECK (aggregate_version >= 1),
    CONSTRAINT outbox_messages_sequence CHECK (sequence >= 1),
    CONSTRAINT outbox_messages_event_type CHECK (
        char_length(event_type) <= 128
        AND event_type ~ '^workspace\.thinkpixel\.io/[a-z0-9]+(-[a-z0-9]+)*\.[a-z0-9]+(-[a-z0-9]+)*\.v[1-9][0-9]*$'
    ),
    CONSTRAINT outbox_messages_event_version CHECK (
        event_version >= 1 AND event_type ~ ('\.v' || event_version::text || '$')
    ),
    CONSTRAINT outbox_messages_payload_schema CHECK (
        payload_schema = btrim(payload_schema) AND payload_schema <> ''
        AND char_length(payload_schema) <= 128 AND payload_schema !~ '[[:cntrl:]]'
    ),
    CONSTRAINT outbox_messages_payload_object CHECK (
        jsonb_typeof(payload) = 'object' AND octet_length(payload::text) <= 65536
    ),
    CONSTRAINT outbox_messages_attempts CHECK (attempts >= 0),
    CONSTRAINT outbox_messages_timestamps CHECK (
        available_at >= occurred_at AND (published_at IS NULL OR published_at >= occurred_at)
    )
);

CREATE UNIQUE INDEX outbox_messages_aggregate_sequence_unique
    ON thinkpixelws.outbox_messages (tenant_id, aggregate_kind, aggregate_id, sequence);

CREATE INDEX outbox_messages_pending_idx
    ON thinkpixelws.outbox_messages (available_at, occurred_at, event_id)
    WHERE published_at IS NULL;

CREATE INDEX outbox_messages_transaction_idx
    ON thinkpixelws.outbox_messages (tenant_id, transaction_id, event_id);

COMMENT ON TABLE thinkpixelws.outbox_messages IS
    'Tenant-scoped transactional outbox for at-least-once event delivery; consumers deduplicate by event_id.';
COMMENT ON COLUMN thinkpixelws.outbox_messages.transaction_id IS
    'Application transaction identity shared by the business mutation and its outbox message.';
COMMENT ON COLUMN thinkpixelws.outbox_messages.payload IS
    'Schema-versioned policy-safe metadata only; never content, credentials, profile handles, signed URLs, or authority.';
