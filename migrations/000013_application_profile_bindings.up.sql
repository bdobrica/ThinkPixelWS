CREATE TABLE thinkpixelws.profile_bindings (
    tenant_id uuid NOT NULL,
    workspace_id uuid NOT NULL,
    name text NOT NULL,
    kind text NOT NULL,
    profile_ref text NOT NULL,
    retention_policy text,
    created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,

    PRIMARY KEY (tenant_id, workspace_id, name),
    CONSTRAINT profile_bindings_workspace_fk
        FOREIGN KEY (tenant_id, workspace_id)
        REFERENCES thinkpixelws.workspaces (tenant_id, workspace_id),
    CONSTRAINT profile_bindings_name_format CHECK (
        name ~ '^[a-z]([a-z0-9-]{0,61}[a-z0-9])?$'
    ),
    CONSTRAINT profile_bindings_kind CHECK (
        kind IN ('browser', 'desktop', 'application', 'ide')
    ),
    CONSTRAINT profile_bindings_ref CHECK (
        profile_ref = btrim(profile_ref)
        AND profile_ref <> ''
        AND char_length(profile_ref) <= 2048
        AND profile_ref !~ '[[:cntrl:]]'
        AND profile_ref ~ '^[A-Za-z][A-Za-z0-9+.-]*:'
        AND profile_ref !~ '^[A-Za-z][A-Za-z0-9+.-]*://[^/?#]*@'
    ),
    CONSTRAINT profile_bindings_retention_policy CHECK (
        retention_policy IS NULL
        OR (
            retention_policy = btrim(retention_policy)
            AND retention_policy <> ''
            AND char_length(retention_policy) <= 128
            AND retention_policy !~ '[[:cntrl:]]'
        )
    )
);

COMMENT ON TABLE thinkpixelws.profile_bindings IS
    'Credential-adjacent application profile references only; contains no profile data, credential, handle, or access grant.';
COMMENT ON COLUMN thinkpixelws.profile_bindings.profile_ref IS
    'Opaque credential-free ProfileProvider reference; possession does not authorize profile access.';
COMMENT ON COLUMN thinkpixelws.profile_bindings.retention_policy IS
    'Optional policy identifier, not an authorization decision or grant.';
