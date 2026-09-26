ALTER TABLE thinkpixelws.workspace_generations
    ADD COLUMN created_by_run_id uuid,
    ADD CONSTRAINT workspace_generations_run_id_uuidv7 CHECK (
        created_by_run_id IS NULL
        OR (get_byte(uuid_send(created_by_run_id), 6) >> 4) = 7
    );

COMMENT ON COLUMN thinkpixelws.workspace_generations.created_by_run_id IS
    'Optional initiating AG Run identity from trusted orchestration; attribution does not grant authority.';
