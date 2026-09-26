CREATE TABLE thinkpixelws.materializations (
    tenant_id uuid NOT NULL,
    materialization_id uuid NOT NULL,
    workspace_id uuid NOT NULL,
    base_generation bigint NOT NULL,
    provider text NOT NULL,
    target_id text NOT NULL,
    target_region text NOT NULL,
    target_storage_class text NOT NULL,
    target_architecture text,
    mode text NOT NULL,
    lifecycle_state text NOT NULL,
    state_version bigint NOT NULL DEFAULT 1,
    created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,

    PRIMARY KEY (tenant_id, materialization_id),
    CONSTRAINT materializations_generation_fk
        FOREIGN KEY (tenant_id, workspace_id, base_generation)
        REFERENCES thinkpixelws.workspace_generations (tenant_id, workspace_id, generation),
    CONSTRAINT materializations_id_uuidv7 CHECK (
        (get_byte(uuid_send(materialization_id), 6) >> 4) = 7
        AND (get_byte(uuid_send(materialization_id), 8) & 192) = 128
    ),
    CONSTRAINT materializations_generation_positive CHECK (base_generation >= 1),
    CONSTRAINT materializations_provider CHECK (provider ~ '^[a-z][a-z0-9.-]{0,62}$'),
    CONSTRAINT materializations_target_id CHECK (
        target_id = btrim(target_id) AND char_length(target_id) BETWEEN 1 AND 256
        AND target_id !~ '[[:cntrl:]]'
    ),
    CONSTRAINT materializations_target_region CHECK (
        target_region = btrim(target_region) AND char_length(target_region) BETWEEN 1 AND 63
        AND target_region !~ '[[:cntrl:]]'
    ),
    CONSTRAINT materializations_target_storage_class CHECK (
        target_storage_class = btrim(target_storage_class) AND char_length(target_storage_class) BETWEEN 1 AND 253
        AND target_storage_class !~ '[[:cntrl:]]'
    ),
    CONSTRAINT materializations_target_architecture CHECK (
        target_architecture IS NULL OR (
            target_architecture = btrim(target_architecture) AND char_length(target_architecture) BETWEEN 1 AND 32
            AND target_architecture !~ '[[:cntrl:]]'
        )
    ),
    CONSTRAINT materializations_mode CHECK (mode IN ('read-only', 'read-write')),
    CONSTRAINT materializations_state CHECK (lifecycle_state IN (
        'REQUESTED', 'PREPARING', 'READY', 'ACTIVE', 'CHECKPOINTING',
        'RELEASING', 'RELEASED', 'FAILED', 'FENCED'
    )),
    CONSTRAINT materializations_state_version_positive CHECK (state_version >= 1),
    CONSTRAINT materializations_timestamps CHECK (
        isfinite(created_at) AND isfinite(updated_at) AND updated_at >= created_at
    )
);

CREATE INDEX materializations_workspace_idx
    ON thinkpixelws.materializations (tenant_id, workspace_id, materialization_id);

COMMENT ON TABLE thinkpixelws.materializations IS
    'Temporary realizations of completed generations. Requested mode and placement are metadata, never execution authority or a writer lease.';
