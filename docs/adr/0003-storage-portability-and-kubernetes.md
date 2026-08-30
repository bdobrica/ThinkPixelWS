# ADR-0003: Storage portability and Kubernetes Materializations

- Status: Accepted
- Date: 2026-08-30

## Context

Hot CSI storage is useful for working performance but is neither a portable identity nor sufficient disaster-recovery state.

## Decision

PostgreSQL is authoritative for control metadata. Kubernetes CSI PVCs provide the first hot `MaterializationProvider`; CSI `VolumeSnapshot` is an optional provider checkpoint. No custom Workspace CRD/operator is introduced for the RC. An encrypted, versioned, content-addressed portable snapshot stored through an object-store `PortableStore` is canonical for portable committed content.

The portable manifest identifies immutable blobs by SHA-256, canonical JSON manifest digest, byte length, media type, and component-relative path. Implementations may pack blobs for transport, but packs are indexes/containers rather than identity. AES-256-GCM envelope encryption uses a random data key wrapped by an external `KeyProvider`; raw keys never enter PostgreSQL, events, or logs.

## Consequences

Roaming means reconstruction from portable state on a different compatible target without referencing the original volume. Provider checkpoints improve RPO but cannot substantiate roaming. Kubernetes and CSI capability discovery is required at startup and before operations.

## Alternatives considered

- PVC/VolumeSnapshot as canonical state: rejected as backend and cluster coupled.
- A single tar archive: rejected because it lacks efficient deduplication and partial verification.
- A new distributed filesystem: rejected as outside product scope.

## Verification

See `portable-snapshot-evaluation.md`, snapshot schemas, integrity rules, and Phase 6 cross-target tests.
