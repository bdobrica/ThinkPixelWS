CREATE FUNCTION thinkpixelws.reject_completed_generation_mutation()
RETURNS trigger
LANGUAGE plpgsql
AS $function$
BEGIN
    RAISE EXCEPTION USING
        ERRCODE = '55000',
        MESSAGE = 'completed workspace generations are immutable';
END;
$function$;

CREATE TRIGGER workspace_generations_completed_immutable
BEFORE UPDATE OR DELETE ON thinkpixelws.workspace_generations
FOR EACH ROW
WHEN (OLD.state = 'COMPLETED')
EXECUTE FUNCTION thinkpixelws.reject_completed_generation_mutation();

COMMENT ON FUNCTION thinkpixelws.reject_completed_generation_mutation() IS
    'Rejects updates and deletes of completed WorkspaceGeneration rows.';
