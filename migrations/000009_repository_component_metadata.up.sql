ALTER TABLE thinkpixelws.workspace_components
    ADD CONSTRAINT workspace_components_id_kind_unique
    UNIQUE (tenant_id, workspace_id, component_id, kind);

CREATE TABLE thinkpixelws.repository_component_metadata (
    tenant_id uuid NOT NULL,
    workspace_id uuid NOT NULL,
    component_id uuid NOT NULL,
    component_kind text NOT NULL DEFAULT 'repository',
    version_control_system text NOT NULL,
    default_ref text,
    created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,

    PRIMARY KEY (tenant_id, workspace_id, component_id),
    CONSTRAINT repository_component_metadata_component_fk
        FOREIGN KEY (tenant_id, workspace_id, component_id, component_kind)
        REFERENCES thinkpixelws.workspace_components (
            tenant_id, workspace_id, component_id, kind
        ),
    CONSTRAINT repository_component_metadata_kind CHECK (
        component_kind = 'repository'
    ),
    CONSTRAINT repository_component_metadata_vcs CHECK (
        version_control_system = btrim(version_control_system)
        AND version_control_system <> ''
        AND char_length(version_control_system) <= 64
        AND version_control_system ~ '^[a-z][a-z0-9-]*$'
    ),
    CONSTRAINT repository_component_metadata_default_ref CHECK (
        default_ref IS NULL
        OR (
            default_ref = btrim(default_ref)
            AND default_ref <> ''
            AND char_length(default_ref) <= 512
            AND default_ref !~ '[[:cntrl:]]'
        )
    )
);

COMMENT ON TABLE thinkpixelws.repository_component_metadata IS
    'Repository-specific metadata; source identity/revisions, provenance, credentials, and generation state are stored separately.';
COMMENT ON COLUMN thinkpixelws.repository_component_metadata.default_ref IS
    'Optional descriptive default branch or ref; not an immutable resolved source revision.';
