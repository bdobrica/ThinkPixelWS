CREATE TABLE thinkpixelws.workspaces (
    tenant_id uuid NOT NULL,
    workspace_id uuid NOT NULL,
    name text NOT NULL,
    description text,
    owner_kind text NOT NULL,
    owner_id text NOT NULL,
    lifecycle_state text NOT NULL,
    state_version bigint NOT NULL DEFAULT 1,
    writer_fence bigint NOT NULL DEFAULT 0,
    classification text NOT NULL DEFAULT 'internal',
    residency text[] NOT NULL DEFAULT '{}',
    created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,

    PRIMARY KEY (tenant_id, workspace_id),
    CONSTRAINT workspaces_tenant_fk FOREIGN KEY (tenant_id)
        REFERENCES thinkpixelws.tenants (tenant_id),
    CONSTRAINT workspaces_workspace_id_uuidv7 CHECK (
        (get_byte(uuid_send(workspace_id), 6) >> 4) = 7
    ),
    CONSTRAINT workspaces_name_format CHECK (
        name ~ '^[a-z]([a-z0-9-]{0,61}[a-z0-9])?$'
    ),
    CONSTRAINT workspaces_description_length CHECK (
        description IS NULL OR char_length(description) <= 4096
    ),
    CONSTRAINT workspaces_owner_kind CHECK (
        owner_kind IN ('user', 'team', 'service', 'project')
    ),
    CONSTRAINT workspaces_owner_id CHECK (
        owner_id = btrim(owner_id)
        AND owner_id <> ''
        AND char_length(owner_id) <= 256
    ),
    CONSTRAINT workspaces_lifecycle_state CHECK (
        lifecycle_state IN (
            'CREATING', 'READY', 'ARCHIVING', 'ARCHIVED', 'RESTORING',
            'DELETING', 'DELETED', 'DEGRADED'
        )
    ),
    CONSTRAINT workspaces_state_version_positive CHECK (state_version >= 1),
    CONSTRAINT workspaces_writer_fence_nonnegative CHECK (writer_fence >= 0),
    CONSTRAINT workspaces_classification CHECK (
        classification IN ('public', 'internal', 'confidential', 'restricted')
    ),
    CONSTRAINT workspaces_residency_cardinality CHECK (
        cardinality(residency) <= 64
    ),
    CONSTRAINT workspaces_updated_after_created CHECK (updated_at >= created_at)
);

COMMENT ON TABLE thinkpixelws.workspaces IS
    'Tenant-scoped logical Workspace identities; ownership and membership metadata do not confer authority.';
COMMENT ON COLUMN thinkpixelws.workspaces.owner_id IS
    'Administrative owner reference used as authorization context, never as proof of authority.';
COMMENT ON COLUMN thinkpixelws.workspaces.residency IS
    'Allowed placement labels; locality metadata is non-authorizing.';
