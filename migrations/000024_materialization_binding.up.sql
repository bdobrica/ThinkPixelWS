ALTER TABLE thinkpixelws.materializations
    ADD COLUMN provider_handle text,
    ADD CONSTRAINT materializations_provider_handle CHECK (
        provider_handle IS NULL OR (
            lifecycle_state <> 'REQUESTED'
            AND provider_handle = btrim(provider_handle)
            AND char_length(provider_handle) BETWEEN 1 AND 4096
            AND provider_handle !~ '[[:cntrl:]]'
        )
    );

COMMENT ON COLUMN thinkpixelws.materializations.provider_handle IS
    'Opaque non-authorizing provider reference; no credentials or grants. Retained for cleanup after termination.';
