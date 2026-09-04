CREATE SCHEMA thinkpixelws;

REVOKE ALL ON SCHEMA thinkpixelws FROM PUBLIC;

CREATE TABLE thinkpixelws.tenants (
    tenant_id uuid PRIMARY KEY,
    lifecycle_state text NOT NULL,
    administrative_policy_ref text,
    created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,

    CONSTRAINT tenants_tenant_id_uuidv7 CHECK (
        (get_byte(uuid_send(tenant_id), 6) >> 4) = 7
    ),
    CONSTRAINT tenants_lifecycle_state_not_blank CHECK (
        lifecycle_state = btrim(lifecycle_state)
        AND lifecycle_state <> ''
    ),
    CONSTRAINT tenants_administrative_policy_ref_not_blank CHECK (
        administrative_policy_ref IS NULL
        OR (
            administrative_policy_ref = btrim(administrative_policy_ref)
            AND administrative_policy_ref <> ''
        )
    ),
    CONSTRAINT tenants_updated_after_created CHECK (updated_at >= created_at)
);

COMMENT ON TABLE thinkpixelws.tenants IS
    'Root records for tenant-scoped ThinkPixelWS metadata; references do not confer authority.';
COMMENT ON COLUMN thinkpixelws.tenants.administrative_policy_ref IS
    'Opaque reference to administrative policy; never a credential or access grant.';
