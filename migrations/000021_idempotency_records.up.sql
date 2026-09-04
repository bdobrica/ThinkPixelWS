CREATE TABLE thinkpixelws.idempotency_records (
    tenant_id uuid NOT NULL,
    principal text NOT NULL,
    operation text NOT NULL,
    key_hash text NOT NULL,
    request_digest text NOT NULL,
    status text NOT NULL,
    response_status integer,
    result_ref text,
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL,
    expires_at timestamptz NOT NULL,

    PRIMARY KEY (tenant_id, principal, operation, key_hash),
    CONSTRAINT idempotency_records_tenant_fk FOREIGN KEY (tenant_id)
        REFERENCES thinkpixelws.tenants (tenant_id),
    CONSTRAINT idempotency_records_principal CHECK (
        principal = btrim(principal) AND principal <> ''
        AND char_length(principal) <= 256 AND principal !~ '[[:cntrl:]]'
    ),
    CONSTRAINT idempotency_records_operation CHECK (
        operation = btrim(operation) AND operation <> ''
        AND char_length(operation) <= 128 AND operation !~ '[[:cntrl:]]'
    ),
    CONSTRAINT idempotency_records_key_hash CHECK (
        key_hash ~ '^sha256:[0-9a-f]{64}$' AND key_hash <> ('sha256:' || repeat('0', 64))
    ),
    CONSTRAINT idempotency_records_request_digest CHECK (
        request_digest ~ '^sha256:[0-9a-f]{64}$' AND request_digest <> ('sha256:' || repeat('0', 64))
    ),
    CONSTRAINT idempotency_records_status CHECK (status IN ('IN_PROGRESS', 'COMPLETED')),
    CONSTRAINT idempotency_records_result CHECK (
        (status = 'IN_PROGRESS' AND response_status IS NULL AND result_ref IS NULL)
        OR (status = 'COMPLETED' AND response_status BETWEEN 100 AND 599
            AND result_ref IS NOT NULL
            AND result_ref = btrim(result_ref) AND result_ref <> ''
            AND char_length(result_ref) <= 512 AND result_ref !~ '[[:cntrl:]]')
    ),
    CONSTRAINT idempotency_records_timestamps CHECK (
        updated_at >= created_at AND expires_at > created_at
    )
);

CREATE INDEX idempotency_records_expiry_idx
    ON thinkpixelws.idempotency_records (expires_at);

COMMENT ON TABLE thinkpixelws.idempotency_records IS
    'Coordinates mutation retries by tenant, principal, operation, and hashed Idempotency-Key.';
COMMENT ON COLUMN thinkpixelws.idempotency_records.key_hash IS
    'SHA-256 digest only; the caller-supplied Idempotency-Key is not persisted.';
COMMENT ON COLUMN thinkpixelws.idempotency_records.request_digest IS
    'Digest of the canonical method, route, tenant, principal, and JSON request body.';
COMMENT ON COLUMN thinkpixelws.idempotency_records.result_ref IS
    'Opaque result resource reference only; never content, credentials, signed URLs, or authority.';
