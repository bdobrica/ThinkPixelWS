# ADR-0002: Immutable generations, disposable Materializations, and fencing

- Status: Accepted
- Date: 2026-08-30

## Context

Workspaces require reproducibility and safe recovery while active working state remains mutable.

## Decision

Completed WorkspaceGenerations are immutable. A Materialization realizes exactly one base generation and is disposable. At most one unexpired writable lease exists per Workspace by default; any number of authorized read-only Materializations may exist. Every writer acquisition increments a Workspace-scoped monotonic fencing token.

The default lease duration is 60 seconds and renewal interval is 20 seconds. A commit supplies the lease ID, fence, and expected head generation. It succeeds only if the lease is current and unexpired, its fence equals the Workspace writer fence, and the expected head equals the current head. Head advancement and generation creation are one serializable database transaction.

A stale writer transitions to `FENCED`, loses renewal eligibility, and cannot checkpoint as authoritative state or commit. It may be retained read-only for recovery under an explicit administrative operation.

## Consequences

Conflicts fail explicitly with RFC 7807 responses. Forking is the preferred parallel-write primitive. Provider snapshots alone never advance Workspace head.

## Alternatives considered

- Multi-master writes: rejected for the RC because filesystem-level merging is not safe or general.
- Lease without fencing: rejected because a paused writer can return after expiry.
- Mutable generations: rejected because audit and restoration would be ambiguous.

## Verification

Database constraints and concurrency tests in later phases must prove single-writer acquisition, stale-fence rejection, and atomic head advancement.
