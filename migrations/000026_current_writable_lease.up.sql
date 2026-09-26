-- Released history does not occupy the writer slot. Expiry alone never removes
-- a row from this index: takeover must explicitly retire it in a transaction.
-- Existing duplicates fail migration rather than silently choosing a writer.
CREATE UNIQUE INDEX materialization_leases_current_writer
    ON thinkpixelws.materialization_leases (tenant_id, workspace_id)
    WHERE released_at IS NULL;
