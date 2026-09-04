CREATE FUNCTION thinkpixelws.enforce_monotonic_workspace_generation()
RETURNS trigger
LANGUAGE plpgsql
AS $function$
DECLARE
    expected_generation bigint;
BEGIN
    PERFORM 1
    FROM thinkpixelws.workspaces
    WHERE tenant_id = NEW.tenant_id
      AND workspace_id = NEW.workspace_id
    FOR UPDATE;

    IF NOT FOUND THEN
        RETURN NEW;
    END IF;

    SELECT COALESCE(MAX(generation), 0) + 1
    INTO expected_generation
    FROM thinkpixelws.workspace_generations
    WHERE tenant_id = NEW.tenant_id
      AND workspace_id = NEW.workspace_id;

    IF NEW.generation <> expected_generation THEN
        RAISE EXCEPTION USING
            ERRCODE = '23514',
            MESSAGE = format(
                'workspace generation must be the next monotonic number: expected %s, got %s',
                expected_generation,
                NEW.generation
            );
    END IF;

    RETURN NEW;
END;
$function$;

CREATE TRIGGER workspace_generations_monotonic
BEFORE INSERT ON thinkpixelws.workspace_generations
FOR EACH ROW
EXECUTE FUNCTION thinkpixelws.enforce_monotonic_workspace_generation();

COMMENT ON FUNCTION thinkpixelws.enforce_monotonic_workspace_generation() IS
    'Locks the owning Workspace and requires each generation number to be the next Workspace-scoped value.';
