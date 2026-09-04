ALTER TABLE thinkpixelws.workspaces
    ADD COLUMN head_generation bigint,
    ADD CONSTRAINT workspaces_head_generation_fk
        FOREIGN KEY (tenant_id, workspace_id, head_generation)
        REFERENCES thinkpixelws.workspace_generations (
            tenant_id,
            workspace_id,
            generation
        );

COMMENT ON COLUMN thinkpixelws.workspaces.head_generation IS
    'Current committed generation; null until the Workspace has its first completed generation.';
