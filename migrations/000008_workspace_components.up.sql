CREATE TABLE thinkpixelws.workspace_components (
    tenant_id uuid NOT NULL,
    workspace_id uuid NOT NULL,
    component_id uuid NOT NULL,
    name text NOT NULL,
    kind text NOT NULL,
    canonical_path text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,

    PRIMARY KEY (tenant_id, workspace_id, component_id),
    CONSTRAINT workspace_components_workspace_fk
        FOREIGN KEY (tenant_id, workspace_id)
        REFERENCES thinkpixelws.workspaces (tenant_id, workspace_id),
    CONSTRAINT workspace_components_component_id_uuidv7 CHECK (
        (get_byte(uuid_send(component_id), 6) >> 4) = 7
    ),
    CONSTRAINT workspace_components_name_format CHECK (
        name ~ '^[a-z]([a-z0-9-]{0,61}[a-z0-9])?$'
    ),
    CONSTRAINT workspace_components_kind CHECK (
        kind IN (
            'repository', 'directory', 'document-collection',
            'artifact-collection'
        )
    ),
    CONSTRAINT workspace_components_canonical_path CHECK (
        canonical_path = '/workspace/' || name
    ),
    CONSTRAINT workspace_components_name_unique
        UNIQUE (tenant_id, workspace_id, name),
    CONSTRAINT workspace_components_path_unique
        UNIQUE (tenant_id, workspace_id, canonical_path),
    CONSTRAINT workspace_components_id_unique
        UNIQUE (tenant_id, component_id)
);

COMMENT ON TABLE thinkpixelws.workspace_components IS
    'Stable tenant-scoped component identities; generation-specific state and provenance are stored separately.';
COMMENT ON COLUMN thinkpixelws.workspace_components.canonical_path IS
    'Canonical deterministic component root, exactly /workspace/<name>.';
