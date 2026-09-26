# PostgreSQL metadata model

PostgreSQL 17 is authoritative for control metadata; large content is stored by providers. All tenant-owned tables include `tenant_id`, and foreign keys include tenant identity to prevent cross-tenant references. UUIDv7 is generated in the application until PostgreSQL support is selected and tested.

## Core relations

| Relation | Important fields and constraints |
|---|---|
| `tenants` | `tenant_id PK`, lifecycle and policy refs |
| `workspaces` | `(tenant_id, workspace_id) PK`, owner kind/id, lifecycle, `state_version`, `head_generation`, `writer_fence`, classification, residency, retention; unique tenant/name where policy requires |
| `workspace_generations` | `(tenant_id, workspace_id, generation) PK`, UUID, parent/fork lineage, state, manifest digest, durability, creator/time; completed rows immutable |
| `workspace_components` | stable component identity, kind, normalized name/path; unique collision key per Workspace |
| `component_generations` | generation-scoped content/snapshot digest and binding metadata; append-only after generation completion |
| `provenance` | immutable source identity/revision, import actor/Run/time, derived-from JSON graph refs, classification, trust, taints |
| `source_bindings` | component, provider/ref, mode, last resolved revision; no credentials |
| `external_bindings` | kind/ref/mode and classification; no grant/token |
| `profile_bindings` | opaque profile ref/kind/policy only; no profile data/credential |
| `environment_bindings` | immutable artifact/spec ref, digest, platform requirements |
| `artifact_bindings` | digest, media type, size, classification, provenance ref |
| `materializations` | Workspace/base generation, provider/target opaque refs, mode/state, current checkpoint, dirty flag, AG Run/AR Execution refs |
| `materialization_leases` | lease UUID, Workspace/Materialization, fence, holder, issued/renewed/expires/released; partial unique current-writer constraint |
| `checkpoints` | provider-local opaque ref, fence, status, digest metadata, timestamps |
| `portable_snapshots` | generation, format/version, manifest digest, object ref, key ref, residency, size/status |
| `workspace_forks` | source Workspace/generation → child Workspace/generation, creator/time |
| `retention_policies` | archive/delete/snapshot rules and legal-hold state/ref |
| `workspace_events` | append-only ordered per Workspace sequence, UUID, type/version, subject refs, safe payload, time |
| `audit_events` | append-only actor/action/target/decision/outcome/request/trace/time; required transaction UUID couples the record to its business mutation; safe metadata only |
| `idempotency_records` | tenant, principal, operation, key hash, request digest, status/result ref/expiry; unique scope tuple |
| `outbox_messages` | event UUID, aggregate/order, type/version, safe payload, attempts/availability/published time |

## Transactional invariants

Migration `000023` implements the core `materializations` relation: tenant-scoped
identity, completed base-generation reference, provider, requested target, mode,
state/version, and creation/update timestamps. The repository creates only
`REQUESTED` records and reads by tenant and Materialization ID; it accepts a
shared SQL transaction for future atomic business/audit/outbox operations.
Lifecycle transitions compare tenant, current state, expected version, and update
time atomically, incrementing the version on success. RELEASED, FAILED, and FENCED
are terminal; stale or mismatched updates return a conflict.
Multiple read-only Materializations may share a Workspace, completed generation,
and target, including while a writable lease exists. Each has its own identity,
binding, and lifecycle; creating, activating, or releasing readers neither consumes
a writer slot nor advances the Workspace fence. These metadata operations require
caller authorization; provider-side read-only mount enforcement remains provider work.
A `read-write` request or lifecycle transition grants no writer lease or execution
authority. Migration `000024` adds an optional, opaque provider handle bounded to
4096 characters. The repository binds it once during PREPARING using tenant and
version compare-and-swap, incrementing the shared state version. Handles survive
terminal transitions for cleanup; replacing a binding requires a new Materialization.
Trusted providers supply non-authorizing references, never credentials or grants.
Rolling back `000024` discards handles while preserving core Materialization metadata.
Migration `000025` adds writable `materialization_leases` metadata: UUIDv7 lease
identity, holder reference, positive fencing token, issued/renewed/expiry times,
and optional release time. A composite foreign key binds the lease to the same
tenant, Workspace, and writable Materialization, preventing read-only references.
The domain constructor uses the ADR-0002 60-second duration; its renewal interval
constant is 20 seconds. Timestamps must be ordered, and release may follow expiry.
Rolling back `000025` discards leases but preserves Materializations and generations.
The Workspace repository atomically increments the existing `writer_fence` under
its row lock and returns the new token. Each Workspace starts at zero; committed
increments produce distinct increasing tokens through PostgreSQL's maximum bigint.
Exhaustion fails without wrapping. Allocation preserves lifecycle state/version and
timestamps. Lease acquisition must use a transaction-backed repository and insert
the lease in that same transaction; a rolled-back token must never be published.
This primitive does not acquire a lease or grant authority.
Migration `000026` adds a partial unique index on `(tenant_id, workspace_id)` for
leases with `released_at IS NULL`. Existing duplicate slots fail migration rather
than silently selecting a writer. Rolling back the index preserves lease history.
The lease repository acquires a slot and allocates its fence in one transaction,
locking the Workspace before the Materialization. Initial acquisition accepts
writable REQUESTED/PREPARING/READY Materializations; callers must separately check
Workspace eligibility and governed authorization. Database wall time after lock
acquisition starts the 60-second lease. Conflicts and insert failures roll back the
fence; results are returned only after commit. Released history does not occupy a
slot. Expired but unreleased leases still block acquisition until MAT-008 implements
retirement/renewal; checkpoint commit fencing remains MAT-009. Provider provisioning/AR attachment, checkpoint/dirty
status, and execution references also remain subsequent work.

- Every repository method requires an explicit tenant context and applies it in predicates; database roles/RLS are defense in depth.
- Generation numbers and event sequence numbers are allocated while locking the Workspace row.
- A trigger or revoked update/delete privileges reject mutation/deletion of completed generation, component-generation, and provenance rows.
- Writer acquisition locks the Workspace, increments `writer_fence`, and inserts the only unexpired active writer lease. A partial unique index covers unreleased leases; expiry retirement remains pending.
- Commit locks Workspace and lease, validates expected head/fence/expiry, inserts the generation graph, advances head, and inserts audit/outbox records atomically.
- State-changing operations update `state_version` with expected-version compare-and-swap.
- Audit/outbox payloads contain identifiers and policy-safe metadata only.

## Migration strategy

Migrations are ordered, transactional where PostgreSQL permits, forward-only, checksum-verified, and immutable after release. Expand/contract changes support rolling compatibility. Destructive contraction requires a later release after readers/writers have migrated. A dedicated migration identity owns DDL; the service role cannot alter schema. Backfills are restartable and bounded. Every release tests empty installation and upgrade from each supported predecessor, and backs up metadata before irreversible steps.

## Idempotency and outbox

Request digests use canonical method, route, tenant/principal, and canonical JSON body. Reusing a key with a different digest returns conflict. In-progress records coordinate concurrent duplicates; completed records return the original status and resource reference. Mutation, audit, and outbox rows share the business transaction. Delivery is at least once and consumers deduplicate by event UUID.
