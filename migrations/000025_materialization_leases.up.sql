-- Include mode in the reference so a lease cannot point at a read-only realization.
ALTER TABLE thinkpixelws.materializations
    ADD CONSTRAINT materializations_lease_reference UNIQUE (tenant_id, workspace_id, materialization_id, mode);

CREATE TABLE thinkpixelws.materialization_leases (
    tenant_id uuid NOT NULL,
    lease_id uuid NOT NULL,
    workspace_id uuid NOT NULL,
    materialization_id uuid NOT NULL,
    materialization_mode text NOT NULL DEFAULT 'read-write',
    fencing_token bigint NOT NULL,
    holder text NOT NULL,
    issued_at timestamptz NOT NULL,
    renewed_at timestamptz NOT NULL,
    expires_at timestamptz NOT NULL,
    released_at timestamptz,

    PRIMARY KEY (tenant_id, lease_id),
    CONSTRAINT materialization_leases_materialization_fk
        FOREIGN KEY (tenant_id, workspace_id, materialization_id, materialization_mode)
        REFERENCES thinkpixelws.materializations (tenant_id, workspace_id, materialization_id, mode),
    CONSTRAINT materialization_leases_writable CHECK (materialization_mode = 'read-write'),
    CONSTRAINT materialization_leases_id_uuidv7 CHECK (
        (get_byte(uuid_send(lease_id), 6) >> 4) = 7
        AND (get_byte(uuid_send(lease_id), 8) & 192) = 128
    ),
    CONSTRAINT materialization_leases_fence_positive CHECK (fencing_token >= 1),
    CONSTRAINT materialization_leases_holder CHECK (
        holder = btrim(holder) AND char_length(holder) BETWEEN 1 AND 256
        AND holder !~ '[[:cntrl:]]'
    ),
    CONSTRAINT materialization_leases_timestamps CHECK (
        isfinite(issued_at) AND isfinite(renewed_at) AND isfinite(expires_at)
        AND renewed_at >= issued_at AND expires_at > renewed_at
        AND (released_at IS NULL OR (isfinite(released_at) AND released_at >= renewed_at))
    )
);

COMMENT ON TABLE thinkpixelws.materialization_leases IS
    'Writable lease metadata. Acquisition must enforce current-writer uniqueness, Workspace fence allocation, and authority separately.';
COMMENT ON COLUMN thinkpixelws.materialization_leases.holder IS
    'Stable non-secret holder reference, never a credential or execution grant.';
