CREATE TABLE thinkpixelws.retention_policies (
    tenant_id uuid NOT NULL,
    workspace_id uuid NOT NULL,
    state_version bigint NOT NULL DEFAULT 1,
    idle_ttl_seconds bigint,
    archive_after_inactivity_seconds bigint,
    snapshot_retention_seconds bigint,
    maximum_generations bigint,
    delete_after_seconds bigint,
    profile_retention_seconds bigint,
    legal_hold boolean NOT NULL DEFAULT false,
    legal_hold_reference text,
    created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,

    PRIMARY KEY (tenant_id, workspace_id),
    CONSTRAINT retention_policies_workspace_fk
        FOREIGN KEY (tenant_id, workspace_id)
        REFERENCES thinkpixelws.workspaces (tenant_id, workspace_id),
    CONSTRAINT retention_policies_state_version_positive CHECK (state_version >= 1),
    CONSTRAINT retention_policies_idle_ttl_positive CHECK (idle_ttl_seconds IS NULL OR idle_ttl_seconds > 0),
    CONSTRAINT retention_policies_archive_after_positive CHECK (archive_after_inactivity_seconds IS NULL OR archive_after_inactivity_seconds > 0),
    CONSTRAINT retention_policies_snapshot_retention_positive CHECK (snapshot_retention_seconds IS NULL OR snapshot_retention_seconds > 0),
    CONSTRAINT retention_policies_maximum_generations_positive CHECK (maximum_generations IS NULL OR maximum_generations > 0),
    CONSTRAINT retention_policies_delete_after_positive CHECK (delete_after_seconds IS NULL OR delete_after_seconds > 0),
    CONSTRAINT retention_policies_profile_retention_positive CHECK (profile_retention_seconds IS NULL OR profile_retention_seconds > 0),
    CONSTRAINT retention_policies_legal_hold_reference CHECK (
        (legal_hold AND legal_hold_reference IS NOT NULL
            AND legal_hold_reference = btrim(legal_hold_reference)
            AND legal_hold_reference <> '' AND char_length(legal_hold_reference) <= 256
            AND legal_hold_reference !~ '[[:cntrl:]]')
        OR (NOT legal_hold AND legal_hold_reference IS NULL)
    ),
    CONSTRAINT retention_policies_updated_after_created CHECK (updated_at >= created_at)
);

COMMENT ON TABLE thinkpixelws.retention_policies IS
    'Tenant-scoped Workspace lifecycle policy metadata; it does not authorize archive, deletion, transfer, or key destruction.';
COMMENT ON COLUMN thinkpixelws.retention_policies.legal_hold IS
    'When true, blocks expiry, archive deletion, snapshot deletion, and crypto-shredding; enforcement remains transactional.';
