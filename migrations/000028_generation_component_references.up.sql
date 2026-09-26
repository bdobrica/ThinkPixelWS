ALTER TABLE thinkpixelws.workspace_generations
    ADD COLUMN component_references jsonb,
    ADD CONSTRAINT workspace_generations_component_references_array CHECK (
        component_references IS NULL OR
        CASE WHEN jsonb_typeof(component_references) = 'array'
        THEN jsonb_array_length(component_references) <= 256 ELSE false END
    );

COMMENT ON COLUMN thinkpixelws.workspace_generations.component_references IS
    'Immutable component snapshot digests or provider/target-scoped checkpoint handles; NULL means references were not recorded by an older writer. Not credentials or authority.';
