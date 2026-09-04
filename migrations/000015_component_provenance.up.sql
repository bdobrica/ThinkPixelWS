CREATE FUNCTION thinkpixelws.valid_provenance_taints(values_to_check text[])
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

CREATE TABLE thinkpixelws.provenance (
    tenant_id uuid NOT NULL,
    workspace_id uuid NOT NULL,
    generation bigint NOT NULL,
    component_id uuid NOT NULL,
    source text NOT NULL,
    source_type text NOT NULL,
    source_revision text NOT NULL,
    imported_at timestamptz NOT NULL,
    initiating_principal text NOT NULL,
    source_run_id uuid,
    derived_from text[] NOT NULL DEFAULT '{}',
    classification text NOT NULL,
    source_trust text NOT NULL,
    taints text[] NOT NULL DEFAULT '{}',

    PRIMARY KEY (tenant_id, workspace_id, generation, component_id),
    CONSTRAINT component_provenance_generation_fk
        FOREIGN KEY (tenant_id, workspace_id, generation)
        REFERENCES thinkpixelws.workspace_generations (tenant_id, workspace_id, generation),
    CONSTRAINT component_provenance_component_fk
        FOREIGN KEY (tenant_id, workspace_id, component_id)
        REFERENCES thinkpixelws.workspace_components (tenant_id, workspace_id, component_id),
    CONSTRAINT component_provenance_source CHECK (
        source = btrim(source) AND source <> '' AND char_length(source) <= 2048
        AND source !~ '[[:cntrl:]]'
        AND source ~ '^[A-Za-z][A-Za-z0-9+.-]*:'
        AND source !~ '^[A-Za-z][A-Za-z0-9+.-]*://[^/?#]*@'
    ),
    CONSTRAINT component_provenance_source_type CHECK (
        source_type = btrim(source_type) AND source_type <> ''
        AND char_length(source_type) <= 64 AND source_type !~ '[[:cntrl:]]'
    ),
    CONSTRAINT component_provenance_source_revision CHECK (
        source_revision = btrim(source_revision) AND source_revision <> ''
        AND char_length(source_revision) <= 512 AND source_revision !~ '[[:cntrl:]]'
    ),
    CONSTRAINT component_provenance_principal CHECK (
        initiating_principal = btrim(initiating_principal)
        AND initiating_principal <> '' AND char_length(initiating_principal) <= 256
        AND initiating_principal !~ '[[:cntrl:]]'
    ),
    CONSTRAINT component_provenance_run_id_uuidv7 CHECK (
        source_run_id IS NULL OR (get_byte(uuid_send(source_run_id), 6) >> 4) = 7
    ),
    CONSTRAINT component_provenance_derived_from CHECK (
        cardinality(derived_from) <= 256
        AND array_position(derived_from, NULL) IS NULL
        AND (
            cardinality(derived_from) = 0
            OR array_to_string(derived_from, ',') ~ '^(sha256:[0-9a-f]{64})(,sha256:[0-9a-f]{64})*$'
        )
    ),
    CONSTRAINT component_provenance_classification CHECK (
        classification IN ('public', 'internal', 'confidential', 'restricted')
    ),
    CONSTRAINT component_provenance_source_trust CHECK (
        source_trust IN ('trusted-internal', 'authenticated-external', 'external-untrusted')
    ),
    CONSTRAINT component_provenance_taints CHECK (
        thinkpixelws.valid_provenance_taints(taints)
    )
);

CREATE FUNCTION thinkpixelws.reject_component_provenance_mutation()
RETURNS trigger
LANGUAGE plpgsql
AS $function$
BEGIN
    RAISE EXCEPTION USING ERRCODE = '55000', MESSAGE = 'component provenance is immutable';
END;
$function$;

CREATE TRIGGER component_provenance_immutable
BEFORE UPDATE OR DELETE ON thinkpixelws.provenance
FOR EACH ROW EXECUTE FUNCTION thinkpixelws.reject_component_provenance_mutation();

COMMENT ON TABLE thinkpixelws.provenance IS
    'Immutable tenant-scoped origin, derivation, classification, and trust evidence for a component generation.';
COMMENT ON COLUMN thinkpixelws.provenance.source IS
    'Credential-free absolute source identity; provenance is evidence and grants no source or Run authority.';
COMMENT ON FUNCTION thinkpixelws.valid_provenance_taints(text[]) IS
    'Validates bounded, control-free, unique provenance taint labels.';
