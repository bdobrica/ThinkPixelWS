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
| `audit_events` | append-only actor/action/target/decision/outcome/request/trace/time; safe metadata only |
| `idempotency_records` | tenant, principal, operation, key hash, request digest, status/result ref/expiry; unique scope tuple |
| `outbox_messages` | event UUID, aggregate/order, type/version, safe payload, attempts/availability/published time |

## Transactional invariants

- Every repository method requires an explicit tenant context and applies it in predicates; database roles/RLS are defense in depth.
- Generation numbers and event sequence numbers are allocated while locking the Workspace row.
- A trigger or revoked update/delete privileges reject mutation/deletion of completed generation, component-generation, and provenance rows.
- Writer acquisition locks the Workspace, increments `writer_fence`, and inserts the only unexpired active writer lease. A partial unique index covers active lease status; serializable retry handles expiry races.
- Commit locks Workspace and lease, validates expected head/fence/expiry, inserts the generation graph, advances head, and inserts audit/outbox records atomically.
- State-changing operations update `state_version` with expected-version compare-and-swap.
- Audit/outbox payloads contain identifiers and policy-safe metadata only.

## Migration strategy

Migrations are ordered, transactional where PostgreSQL permits, forward-only, checksum-verified, and immutable after release. Expand/contract changes support rolling compatibility. Destructive contraction requires a later release after readers/writers have migrated. A dedicated migration identity owns DDL; the service role cannot alter schema. Backfills are restartable and bounded. Every release tests empty installation and upgrade from each supported predecessor, and backs up metadata before irreversible steps.

## Idempotency and outbox

Request digests use canonical method, route, tenant/principal, and canonical JSON body. Reusing a key with a different digest returns conflict. In-progress records coordinate concurrent duplicates; completed records return the original status and resource reference. Mutation, audit, and outbox rows share the business transaction. Delivery is at least once and consumers deduplicate by event UUID.
