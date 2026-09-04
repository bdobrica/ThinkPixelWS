CREATE TABLE thinkpixelws.directory_component_metadata (
    tenant_id uuid NOT NULL,
    workspace_id uuid NOT NULL,
    component_id uuid NOT NULL,
    component_kind text NOT NULL DEFAULT 'directory',
    created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,

    PRIMARY KEY (tenant_id, workspace_id, component_id),
    CONSTRAINT directory_component_metadata_component_fk
        FOREIGN KEY (tenant_id, workspace_id, component_id, component_kind)
        REFERENCES thinkpixelws.workspace_components (
            tenant_id, workspace_id, component_id, kind
        ),
    CONSTRAINT directory_component_metadata_kind CHECK (
        component_kind = 'directory'
    )
);

COMMENT ON TABLE thinkpixelws.directory_component_metadata IS
    'Directory-specific metadata; content state, source bindings, provenance, classification, and credentials are stored separately.';

CREATE TABLE thinkpixelws.document_collection_component_metadata (
    tenant_id uuid NOT NULL,
    workspace_id uuid NOT NULL,
    component_id uuid NOT NULL,
    component_kind text NOT NULL DEFAULT 'document-collection',
    created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,

    PRIMARY KEY (tenant_id, workspace_id, component_id),
    CONSTRAINT document_collection_component_metadata_component_fk
        FOREIGN KEY (tenant_id, workspace_id, component_id, component_kind)
        REFERENCES thinkpixelws.workspace_components (
            tenant_id, workspace_id, component_id, kind
        ),
    CONSTRAINT document_collection_component_metadata_kind CHECK (
        component_kind = 'document-collection'
    )
);

COMMENT ON TABLE thinkpixelws.document_collection_component_metadata IS
    'Document-collection-specific metadata; snapshot state, source bindings, provenance, classification, and credentials are stored separately.';
