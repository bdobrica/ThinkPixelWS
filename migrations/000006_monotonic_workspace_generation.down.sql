DROP TRIGGER workspace_generations_monotonic
    ON thinkpixelws.workspace_generations;
DROP FUNCTION thinkpixelws.enforce_monotonic_workspace_generation();
