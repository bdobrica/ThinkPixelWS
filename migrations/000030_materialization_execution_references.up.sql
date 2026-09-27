ALTER TABLE thinkpixelws.materializations
    ADD COLUMN run_id uuid,
    ADD COLUMN execution_id uuid,
    ADD CONSTRAINT materializations_run_id_uuidv7 CHECK (
        run_id IS NULL OR ((get_byte(uuid_send(run_id), 6) >> 4) = 7
        AND (get_byte(uuid_send(run_id), 8) & 192) = 128)
    ),
    ADD CONSTRAINT materializations_execution_id_uuidv7 CHECK (
        execution_id IS NULL OR (run_id IS NOT NULL
        AND (get_byte(uuid_send(execution_id), 6) >> 4) = 7
        AND (get_byte(uuid_send(execution_id), 8) & 192) = 128)
    );

CREATE FUNCTION thinkpixelws.preserve_materialization_execution_references()
RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF NEW.run_id IS DISTINCT FROM OLD.run_id OR NEW.execution_id IS DISTINCT FROM OLD.execution_id THEN
        RAISE EXCEPTION 'materialization execution references are immutable' USING ERRCODE = '23514';
    END IF;
    RETURN NEW;
END;
$$;

CREATE TRIGGER materialization_execution_references_immutable
BEFORE UPDATE ON thinkpixelws.materializations
FOR EACH ROW EXECUTE FUNCTION thinkpixelws.preserve_materialization_execution_references();

COMMENT ON COLUMN thinkpixelws.materializations.run_id IS
    'AG Run correlation from verified authority for governed requests. NULL for legacy/local records. Not authority.';
COMMENT ON COLUMN thinkpixelws.materializations.execution_id IS
    'Optional AR Execution correlation from verified authority. Not authority; replacement execution needs a new Materialization.';
