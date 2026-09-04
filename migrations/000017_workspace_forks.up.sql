CREATE TABLE thinkpixelws.workspace_forks (
    tenant_id uuid NOT NULL,
    source_workspace_id uuid NOT NULL,
    source_generation bigint NOT NULL,
    child_workspace_id uuid NOT NULL,
    child_generation bigint NOT NULL DEFAULT 1,
    created_by_principal text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,

    PRIMARY KEY (tenant_id, child_workspace_id, child_generation),
    CONSTRAINT workspace_forks_source_generation_fk
        FOREIGN KEY (tenant_id, source_workspace_id, source_generation)
        REFERENCES thinkpixelws.workspace_generations (tenant_id, workspace_id, generation),
    CONSTRAINT workspace_forks_child_generation_fk
        FOREIGN KEY (tenant_id, child_workspace_id, child_generation)
        REFERENCES thinkpixelws.workspace_generations (tenant_id, workspace_id, generation),
    CONSTRAINT workspace_forks_distinct_workspaces CHECK (
        source_workspace_id <> child_workspace_id
    ),
    CONSTRAINT workspace_forks_source_generation_positive CHECK (
        source_generation >= 1
    ),
    CONSTRAINT workspace_forks_child_generation_initial CHECK (
        child_generation = 1
    ),
    CONSTRAINT workspace_forks_creator CHECK (
        created_by_principal = btrim(created_by_principal)
        AND created_by_principal <> ''
        AND char_length(created_by_principal) <= 256
        AND created_by_principal !~ '[[:cntrl:]]'
    )
);

CREATE INDEX workspace_forks_source_generation_idx
    ON thinkpixelws.workspace_forks (tenant_id, source_workspace_id, source_generation);

CREATE FUNCTION thinkpixelws.reject_workspace_fork_mutation()
RETURNS trigger
LANGUAGE plpgsql
AS $function$
BEGIN
    RAISE EXCEPTION USING ERRCODE = '55000', MESSAGE = 'Workspace fork lineage is immutable';
END;
$function$;

CREATE TRIGGER workspace_forks_immutable
BEFORE UPDATE OR DELETE ON thinkpixelws.workspace_forks
FOR EACH ROW EXECUTE FUNCTION thinkpixelws.reject_workspace_fork_mutation();

COMMENT ON TABLE thinkpixelws.workspace_forks IS
    'Immutable tenant-scoped lineage from a completed source generation to generation 1 of a child Workspace.';
COMMENT ON COLUMN thinkpixelws.workspace_forks.created_by_principal IS
    'Fork creator identity for attribution; it grants no Workspace or execution authority.';
