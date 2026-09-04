DROP TABLE thinkpixelws.repository_component_metadata;

ALTER TABLE thinkpixelws.workspace_components
    DROP CONSTRAINT workspace_components_id_kind_unique;
