# Database development

## Generation commit boundary

[`ports.GenerationCommitter`](../internal/ports/generation_committer.go) has a
[PostgreSQL implementation](../internal/adapters/postgres/generation_committer.go)
for publishing prepared content from an ACTIVE or CHECKPOINTING writable
Materialization. The caller supplies tenant/Workspace/Materialization identity,
lease and fence, expected head, captured Materialization state version, stable
generation ID, manifest digest, exact component references, durability, principal
and optional Run/Execution IDs.
Provenance comes from trusted governance/runtime context and conveys no authority;
WS does not query AG or AR databases to validate these identities. Existing
generations retain absent Run provenance rather than inventing attribution.
The adapter creates the next immutable generation, records the previous head as
its parent, advances the head and writes linked audit/outbox records in one
serializable transaction. It checks the writer both before mutation and just
before database commit. Failed publication rolls back all four records.

Migration `000028` stores component references on the immutable generation row.
Each new commit must cover exactly the Workspace's registered component IDs;
foreign, duplicate or missing IDs are rejected. A `portable-snapshot` reference
uses a snapshot manifest SHA-256 digest and component ID. A `provider-checkpoint`
reference uses provider, target and an opaque immutable checkpoint handle, never
a mutable PVC or reusable snapshot name. Portable generations cannot depend on
provider checkpoints. The trusted caller verifies that these references match
the captured manifest and content; SQL does not perform provider IO. Get/list
readback preserves the exact references. Historical generations retain `NULL`
(unknown references); new empty Workspaces record an explicit empty array.

This is an internal metadata operation. Trusted WS orchestration must authorize
the caller under AG governance and capture, persist and verify a complete
immutable manifest and its content before calling it. Provider IO must happen
outside the transaction; a digest supplied by an agent or a mutable PVC reference
is not evidence of a completed capture. The adapter does not inspect blob content
or implement capture, HTTP
idempotency or the public commit endpoint. It preserves Materialization state and
base generation and does not claim that later filesystem writes are clean.

The outbox event `workspace.thinkpixel.io/generation.committed.v1` uses the new
generation ID as its aggregate, version/sequence 1, and payload schema
`generation-committed.v1`. Payload and audit metadata contain only `workspaceId`,
`materializationId`, `generationId` and `generation`. The audit action is
`workspace.commit`; audit and outbox share a transaction ID.

Stale writer/head checks return the existing port conflict errors. PostgreSQL
serialization failures propagate; callers must reconcile the stable generation
ID after an ambiguous database result before retrying. Repeating an already
published expected head fails rather than silently creating another generation.
Do not delete prepared content merely because publication reported an error.

Focused integration tests use a disposable database created by each test and
require a test-only PostgreSQL account with `CREATEDB`:

```sh
THINKPIXELWS_TEST_DATABASE_URL='postgres://.../postgres?sslmode=disable' \
  go test -race ./internal/adapters/postgres \
  -run 'TestGenerationCommit|TestMaterializationWriterGuard' -count=1
```

These tests verify metadata publication, immutable readback, writer/head/version
rejection, competing commits, audit/outbox rollback and expiry during publication.
They do not qualify snapshot capture or sandbox-to-generation recovery.

## Local database setup

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
migration. `make check-migrations` verifies that migration pairs are contiguous
and match the committed SHA-256 manifest. Update the manifest only when adding
an unreleased migration; never replace the checksum of a released migration.

The `thinkpixelws` schema is owned by the migration identity and grants no
access to `PUBLIC`. A separately provisioned service identity receives only the
schema and table privileges needed at runtime; it does not own the schema and
cannot alter it. Role and credential provisioning stays outside migrations so
the same schema works with self-managed and hosted PostgreSQL role models.

Run the Materialization persistence integration test against a disposable
PostgreSQL instance:

```sh
THINKPIXELWS_TEST_DATABASE_URL='postgres://USER:PASSWORD@127.0.0.1:5432/postgres?sslmode=disable' \
  go test -race ./internal/adapters/postgres -run TestMaterializationRepositoryPostgres
```

The test identity needs `CREATE DATABASE` permission. The test creates and
removes its own randomly named database, applies all migrations, and checks
round-trip persistence, tenant isolation, constraints, transaction rollback,
and rollback/reapplication of the Materialization migration. It skips when the
environment variable is unset.
