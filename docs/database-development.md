# Database development

Local database development uses PostgreSQL 17.6 through Docker Compose. The
image and migration tool are pinned by tag and multi-platform digest. PostgreSQL
binds to loopback only and persists data in the `postgres-data` Compose volume.

Start the dependency and apply migrations explicitly:

```sh
make postgres-up
make migrate
```

`make migrate` starts PostgreSQL when necessary, waits for its health check,
and applies pending files from `migrations/`. Service startup does not apply
migrations. This keeps schema changes under an explicit operator/developer
action and leaves room for a separately privileged service identity.

Stop the dependency without deleting its data:

```sh
make postgres-down
```

The development-only defaults are database `thinkpixelws`, migration user
`thinkpixelws_migrator`, password `thinkpixelws-local-only`, and host port
`5432`. Override the password with `THINKPIXELWS_DEV_POSTGRES_PASSWORD` and the
port with `THINKPIXELWS_DEV_POSTGRES_PORT`. These values are local conveniences,
not deployment credentials. Production credentials remain externally managed
secret references as described in [configuration](configuration.md).

Migrations are paired `NNNNNN_name.up.sql` and `NNNNNN_name.down.sql` files.
Once released, a migration is immutable; corrections use a new forward
migration. The schema and database roles begin in DB-001.
