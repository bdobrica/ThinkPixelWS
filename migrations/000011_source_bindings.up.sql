CREATE TABLE thinkpixelws.source_bindings (
    tenant_id uuid NOT NULL,
    workspace_id uuid NOT NULL,
    component_id uuid NOT NULL,
    provider text NOT NULL,
    source_ref text NOT NULL,
    mode text NOT NULL,
    last_resolved_revision text,
    created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,

    PRIMARY KEY (tenant_id, workspace_id, component_id),
    CONSTRAINT source_bindings_component_fk
        FOREIGN KEY (tenant_id, workspace_id, component_id)
        REFERENCES thinkpixelws.workspace_components (
            tenant_id, workspace_id, component_id
        ),
    CONSTRAINT source_bindings_provider CHECK (
        char_length(provider) BETWEEN 1 AND 63
        AND provider ~ '^[a-z][a-z0-9.-]{0,62}$'
    ),
    CONSTRAINT source_bindings_source_ref CHECK (
        source_ref = btrim(source_ref)
        AND source_ref <> ''
        AND char_length(source_ref) <= 2048
        AND source_ref !~ '[[:cntrl:]]'
        AND source_ref ~ '^[A-Za-z][A-Za-z0-9+.-]*:'
        AND source_ref !~ '^[A-Za-z][A-Za-z0-9+.-]*://[^/?#]*@'
    ),
    CONSTRAINT source_bindings_mode CHECK (
        mode IN ('snapshot', 'refreshable-snapshot', 'live-reference')
    ),
    CONSTRAINT source_bindings_last_resolved_revision CHECK (
        last_resolved_revision IS NULL
        OR (
            last_resolved_revision = btrim(last_resolved_revision)
            AND last_resolved_revision <> ''
            AND char_length(last_resolved_revision) <= 512
            AND last_resolved_revision !~ '[[:cntrl:]]'
        )
    )
);

COMMENT ON TABLE thinkpixelws.source_bindings IS
    'Component source acquisition references and modes; contains no credentials, grants, provenance, or classification.';
COMMENT ON COLUMN thinkpixelws.source_bindings.source_ref IS
    'Credential-free provider source reference represented as an absolute URI.';
COMMENT ON COLUMN thinkpixelws.source_bindings.last_resolved_revision IS
    'Most recently resolved provider revision; immutable generation provenance is stored separately.';
