DROP TRIGGER component_provenance_immutable ON thinkpixelws.provenance;
DROP FUNCTION thinkpixelws.reject_component_provenance_mutation();
DROP TABLE thinkpixelws.provenance;
DROP FUNCTION thinkpixelws.valid_provenance_taints(text[]);
