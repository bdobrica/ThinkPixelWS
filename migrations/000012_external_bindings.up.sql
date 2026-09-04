CREATE TABLE thinkpixelws.external_bindings (
    tenant_id uuid NOT NULL,
    workspace_id uuid NOT NULL,
    name text NOT NULL,
    kind text NOT NULL,
    external_ref text NOT NULL,
    mode text NOT NULL,
    classification text,
    created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,

    PRIMARY KEY (tenant_id, workspace_id, name),
    CONSTRAINT external_bindings_workspace_fk
        FOREIGN KEY (tenant_id, workspace_id)
        REFERENCES thinkpixelws.workspaces (tenant_id, workspace_id),
    CONSTRAINT external_bindings_name_format CHECK (
        name ~ '^[a-z]([a-z0-9-]{0,61}[a-z0-9])?$'
    ),
    CONSTRAINT external_bindings_kind CHECK (
        kind IN (
            'collaboration', 'issue-tracker', 'document-store',
            'database', 'service', 'observability'
        )
    ),
    CONSTRAINT external_bindings_ref CHECK (
        external_ref = btrim(external_ref)
        AND external_ref <> ''
        AND char_length(external_ref) <= 2048
        AND external_ref !~ '[[:cntrl:]]'
        AND external_ref ~ '^[A-Za-z][A-Za-z0-9+.-]*:'
        AND external_ref !~ '^[A-Za-z][A-Za-z0-9+.-]*://[^/?#]*@'
    ),
    CONSTRAINT external_bindings_mode CHECK (
        mode IN ('live-reference', 'materialized-copy')
    ),
    CONSTRAINT external_bindings_classification CHECK (
        classification IS NULL
        OR classification IN ('public', 'internal', 'confidential', 'restricted')
    )
);

COMMENT ON TABLE thinkpixelws.external_bindings IS
    'Workspace context references to external resources; descriptive metadata only and never an access grant.';
COMMENT ON COLUMN thinkpixelws.external_bindings.external_ref IS
    'Credential-free external resource reference represented as an absolute URI.';
COMMENT ON COLUMN thinkpixelws.external_bindings.classification IS
    'Optional classification metadata for policy input; it does not confer authority.';
