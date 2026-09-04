DROP TRIGGER workspace_generations_completed_immutable
    ON thinkpixelws.workspace_generations;
DROP FUNCTION thinkpixelws.reject_completed_generation_mutation();
