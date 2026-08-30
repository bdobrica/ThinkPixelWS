# ADR-0005: Lifecycle, retention, and residency

- Status: Accepted
- Date: 2026-08-30

## Decision

Workspace lifecycle is `CREATING → READY → ARCHIVING → ARCHIVED → RESTORING → READY` or `READY|ARCHIVED → DELETING → DELETED`; failures enter observable `DEGRADED` and require retry or administration. Materialization lifecycle is `REQUESTED → PREPARING → READY → ACTIVE → CHECKPOINTING → ACTIVE → RELEASING → RELEASED`, with `FAILED` and `FENCED` terminal paths.

Archive may release hot state only after a verified portable snapshot exists. Delete is asynchronous, tenant-scoped, audited, and removes WS-owned hot state, snapshots, caches, and key references; it never deletes authoritative external sources. Legal hold blocks expiry, archive deletion, snapshot deletion, and crypto-shredding.

Residency is an allow-list of jurisdiction/region/storage-domain labels. Classification policy can narrow it. WS validates destination eligibility and key/profile availability before transfer; AR remains the scheduler. Locality hints are advisory and never authorize placement.

## Consequences

No classified data silently crosses a residency boundary. Retention operations are idempotent and auditable.

## Verification

Lifecycle schemas, policy documentation, audit events, and later integration tests verify these rules.
