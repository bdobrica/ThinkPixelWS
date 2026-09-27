DROP TRIGGER materialization_execution_references_immutable ON thinkpixelws.materializations;
DROP FUNCTION thinkpixelws.preserve_materialization_execution_references();
ALTER TABLE thinkpixelws.materializations DROP COLUMN execution_id, DROP COLUMN run_id;
