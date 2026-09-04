CREATE TABLE thinkpixelws.environment_bindings (
    tenant_id uuid NOT NULL,
    workspace_id uuid NOT NULL,
    kind text NOT NULL,
    environment_ref text NOT NULL,
    digest text,
    platform text,
    created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,

    PRIMARY KEY (tenant_id, workspace_id),
    CONSTRAINT environment_bindings_workspace_fk
        FOREIGN KEY (tenant_id, workspace_id)
        REFERENCES thinkpixelws.workspaces (tenant_id, workspace_id),
    CONSTRAINT environment_bindings_kind CHECK (
        kind IN ('oci', 'thinkpixelmp', 'devcontainer', 'devfile', 'runtime-profile')
    ),
    CONSTRAINT environment_bindings_ref CHECK (
        environment_ref = btrim(environment_ref)
        AND environment_ref <> ''
        AND char_length(environment_ref) <= 2048
        AND environment_ref !~ '[[:cntrl:]]'
        AND environment_ref ~ '^[A-Za-z][A-Za-z0-9+.-]*:'
        AND environment_ref !~ '^[A-Za-z][A-Za-z0-9+.-]*://[^/?#]*@'
    ),
    CONSTRAINT environment_bindings_digest CHECK (
        digest IS NULL OR digest ~ '^sha256:[0-9a-f]{64}$'
    ),
    CONSTRAINT environment_bindings_platform CHECK (
        platform IS NULL
        OR (
            platform = btrim(platform)
            AND platform <> ''
            AND char_length(platform) <= 128
            AND platform !~ '[[:cntrl:]]'
        )
    )
);

COMMENT ON TABLE thinkpixelws.environment_bindings IS
    'Reproducible environment references and untrusted requirements metadata; conveys no runtime privilege or authority.';
COMMENT ON COLUMN thinkpixelws.environment_bindings.environment_ref IS
    'Credential-free environment definition or qualified artifact reference represented as an absolute URI.';
COMMENT ON COLUMN thinkpixelws.environment_bindings.digest IS
    'Optional immutable SHA-256 identity for the referenced definition or artifact.';
COMMENT ON COLUMN thinkpixelws.environment_bindings.platform IS
    'Optional platform requirement interpreted only through trusted policy mapping.';
