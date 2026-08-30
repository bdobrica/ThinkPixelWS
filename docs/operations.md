# Operations, policy, and service objectives

## Placement and residency

Workspace policy contains an allowed set of jurisdiction, region, storage-domain, and optional organization labels. Target context is supplied by AR/operator policy and includes the same labels plus provider capabilities. WS permits materialization/export only when the target is a subset of all Workspace, component, classification, tenant, key, and profile constraints. Missing labels deny transfer.

WS returns locality hints for existing hot copies, portable snapshot regions, source affinity, estimated transfer bytes, and compatible providers. Hints are non-authorizing; AR/schedulers select the target. Every denial and cross-domain transfer is audited.

## Retention, archive, delete, and legal hold

Policies can set idle archive duration, portable/provider snapshot retention, maximum generations, delete-after, and profile-specific retention. Archive first creates and verifies a portable snapshot, then releases hot storage. Restore creates a new Materialization from canonical state. Delete is asynchronous, idempotent, and reports per-provider progress. External sources are dereferenced, never deleted.

A legal hold has authority, case/reference, scope, start time, and optional end time. It blocks automatic/manual deletion, expiry, portable/provider snapshot destruction, key destruction, and retention shortening for in-scope data. Hold creation/removal requires separate administration and immutable audit.

## Workspace events

Event names use `workspace.thinkpixel.io/<noun>.<past-tense>.v1`:

- `workspace.created`, `workspace.ready`, `workspace.archived`, `workspace.restored`, `workspace.deletion-requested`, `workspace.deleted`;
- `component.imported`, `component.refreshed`;
- `generation.committed`;
- `materialization.requested`, `materialization.ready`, `materialization.released`, `materialization.failed`;
- `lease.acquired`, `lease.renewed`, `lease.lost`, `writer.fenced`;
- `checkpoint.completed`, `checkpoint.failed`;
- `portable-snapshot.completed`, `portable-snapshot.failed`;
- `workspace.forked`, `workspace.roamed`;
- `classification.changed`, `residency.denied`;
- `profile.materialized`, `profile.released`, `profile.access-denied`;
- `authority.revoked`, `secret-scan.warning`.

Events include event ID, tenant, aggregate ID/version/sequence, type/version, occurred time, actor/Run/Execution references, trace/request IDs, and a redacted schema-versioned payload. SSE ordering is per Workspace; reconnect uses `Last-Event-ID`. Outbox delivery is at least once.

## Initial service objectives and assumptions

These are Phase 0 targets, not measured guarantees:

| Capability | Target SLO / RPO / RTO assumption |
|---|---|
| Metadata API | 99.9% monthly; p95 reads 250 ms, mutations 750 ms excluding provider work |
| Hot dirty state | RPO last successful checkpoint (target ≤5 min when supported); RTO ≤15 min after sandbox/node loss |
| Committed provider-local generation | RPO 0 after acknowledged commit for surviving storage domain; RTO ≤30 min |
| Portable snapshot | RPO 0 for generations acknowledged `portable`; RTO ≤4 h for ≤100 GiB |
| Materialization | p95 ready ≤10 min for cached ≤20 GiB Workspace; asynchronous above that |
| Archive restore | 99% successful absent policy/KMS/provider outage; RTO ≤4 h for ≤100 GiB |
| Roaming | restore on compatible independent target ≤8 h for ≤100 GiB; exact bytes verified |

Provider/KMS/object-store outages pause affected operations and remain observable. Uncommitted edits beyond the latest checkpoint may be lost. Capacity/performance benchmarks in later phases replace assumptions with measured envelopes.

## Kubernetes/CSI capability contract

The first provider uses Kubernetes API and CSI PVCs; optional checkpoints use external-snapshotter `snapshot.storage.k8s.io/v1`. At startup and operation time, WS discovers StorageClass binding mode, expansion, access modes, topology, clone support, VolumeSnapshotClass and driver match, restore support, capacity errors, and required RBAC. Unsupported optional capabilities return explicit typed results; they do not silently downgrade portable guarantees. Reference integration forbids `hostPath`, privileged containers, host PID/network/IPC, device mounts, cluster-admin/service-account token injection, and arbitrary pod-spec fragments.

WS creates namespaced PVC/VolumeSnapshot resources labeled with opaque tenant/Workspace/Materialization IDs and ownership records. AR receives a provider-neutral attachment handle. AR owns Pod/Sandbox scheduling; WS owns its PVC lifecycle. No custom Workspace CRD/operator is required.
