CREATE TABLE thinkpixelws.workspace_generations (
    tenant_id uuid NOT NULL,
    workspace_id uuid NOT NULL,
    generation bigint NOT NULL,
    generation_id uuid NOT NULL,
    parent_generation bigint,
    state text NOT NULL,
    manifest_digest text NOT NULL,
    durability text NOT NULL,
    created_by_principal text NOT NULL,
    created_by_execution_id uuid,
    created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,

    PRIMARY KEY (tenant_id, workspace_id, generation),
    CONSTRAINT workspace_generations_workspace_fk
        FOREIGN KEY (tenant_id, workspace_id)
        REFERENCES thinkpixelws.workspaces (tenant_id, workspace_id),
    CONSTRAINT workspace_generations_generation_id_unique
        UNIQUE (tenant_id, generation_id),
    CONSTRAINT workspace_generations_generation_id_uuidv7 CHECK (
        (get_byte(uuid_send(generation_id), 6) >> 4) = 7
    ),
    CONSTRAINT workspace_generations_generation_positive CHECK (generation >= 1),
    CONSTRAINT workspace_generations_parent_precedes CHECK (
        parent_generation IS NULL
        OR (parent_generation >= 1 AND parent_generation < generation)
    ),
    CONSTRAINT workspace_generations_state CHECK (state = 'COMPLETED'),
    CONSTRAINT workspace_generations_manifest_digest CHECK (
        manifest_digest ~ '^sha256:[0-9a-f]{64}$'
    ),
    CONSTRAINT workspace_generations_durability CHECK (
        durability IN ('provider-local', 'portable')
    ),
    CONSTRAINT workspace_generations_creator CHECK (
        created_by_principal = btrim(created_by_principal)
        AND created_by_principal <> ''
        AND char_length(created_by_principal) <= 256
    ),
    CONSTRAINT workspace_generations_execution_id_uuidv7 CHECK (
        created_by_execution_id IS NULL
        OR (get_byte(uuid_send(created_by_execution_id), 6) >> 4) = 7
    )
);

COMMENT ON TABLE thinkpixelws.workspace_generations IS
    'Tenant-scoped immutable logical Workspace states; completed-row mutation is enforced by a subsequent migration.';
COMMENT ON COLUMN thinkpixelws.workspace_generations.generation_id IS
    'Stable opaque generation identity; generation is the Workspace-scoped sequence number.';
