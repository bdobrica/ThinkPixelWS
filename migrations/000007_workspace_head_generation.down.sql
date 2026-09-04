ALTER TABLE thinkpixelws.workspaces
    DROP CONSTRAINT workspaces_head_generation_fk,
    DROP COLUMN head_generation;
