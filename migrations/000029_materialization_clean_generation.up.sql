ALTER TABLE thinkpixelws.materializations
    ADD COLUMN clean_generation bigint,
    ADD CONSTRAINT materializations_clean_generation_fk
        FOREIGN KEY (tenant_id, workspace_id, clean_generation)
        REFERENCES thinkpixelws.workspace_generations (tenant_id, workspace_id, generation),
    ADD CONSTRAINT materializations_clean_generation_valid CHECK (
        clean_generation IS NULL OR (
            clean_generation >= 1 AND mode = 'read-write' AND lifecycle_state = 'CHECKPOINTING'
        )
    );

COMMENT ON COLUMN thinkpixelws.materializations.clean_generation IS
    'Generation matching a trusted quiesced capture while CHECKPOINTING. NULL means unknown. Clear before resuming writes; not authority or proof of current head.';
