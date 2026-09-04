CREATE FUNCTION thinkpixelws.valid_classification_taints(values_to_check text[])
RETURNS boolean
LANGUAGE plpgsql
IMMUTABLE
STRICT
AS $function$
DECLARE
    taint text;
BEGIN
    IF array_position(values_to_check, NULL) IS NOT NULL THEN
        RETURN false;
    END IF;
    FOREACH taint IN ARRAY values_to_check LOOP
        IF taint <> btrim(taint) OR taint = '' OR char_length(taint) > 128
            OR taint ~ '[[:cntrl:]]' THEN
            RETURN false;
        END IF;
    END LOOP;
    RETURN cardinality(values_to_check) = (
        SELECT count(DISTINCT value) FROM unnest(values_to_check) AS value
    );
END;
$function$;

CREATE TABLE thinkpixelws.component_classification_metadata (
    tenant_id uuid NOT NULL,
    workspace_id uuid NOT NULL,
    generation bigint NOT NULL,
    component_id uuid NOT NULL,
    classification text NOT NULL,
    taints text[] NOT NULL DEFAULT '{}',

    PRIMARY KEY (tenant_id, workspace_id, generation, component_id),
    CONSTRAINT component_classification_generation_fk
        FOREIGN KEY (tenant_id, workspace_id, generation)
        REFERENCES thinkpixelws.workspace_generations (tenant_id, workspace_id, generation),
    CONSTRAINT component_classification_component_fk
        FOREIGN KEY (tenant_id, workspace_id, component_id)
        REFERENCES thinkpixelws.workspace_components (tenant_id, workspace_id, component_id),
    CONSTRAINT component_classification_value CHECK (
        classification IN ('public', 'internal', 'confidential', 'restricted')
    ),
    CONSTRAINT component_classification_taints CHECK (
        thinkpixelws.valid_classification_taints(taints)
    )
);

CREATE FUNCTION thinkpixelws.reject_component_classification_mutation()
RETURNS trigger
LANGUAGE plpgsql
AS $function$
BEGIN
    RAISE EXCEPTION USING ERRCODE = '55000', MESSAGE = 'component classification metadata is immutable';
END;
$function$;

CREATE TRIGGER component_classification_immutable
BEFORE UPDATE OR DELETE ON thinkpixelws.component_classification_metadata
FOR EACH ROW EXECUTE FUNCTION thinkpixelws.reject_component_classification_mutation();

COMMENT ON TABLE thinkpixelws.component_classification_metadata IS
    'Immutable effective classification and additive taints for a component generation; metadata confers no authority.';
COMMENT ON FUNCTION thinkpixelws.valid_classification_taints(text[]) IS
    'Validates bounded, control-free, unique classification taint labels.';
