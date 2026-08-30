# ADR-0007: API and persistence baseline

- Status: Accepted
- Date: 2026-08-30

## Decision

The canonical API is REST/JSON described by OpenAPI 3.1. OIDC bearer tokens are required except health endpoints. Identifiers are UUIDv7; timestamps are UTC RFC 3339. Errors use RFC 7807. Mutation endpoints require `Idempotency-Key`; list APIs use opaque authenticated cursors; W3C `traceparent` is propagated; event streaming uses resumable SSE and `Last-Event-ID`.

Default limits are 1 MiB JSON bodies, 100 list items (maximum 500), 8 KiB individual headers, 64 KiB aggregate headers, and 30-second ordinary request deadlines. Import/export streams have separately configured byte, file-count, and duration limits.

PostgreSQL is the authoritative metadata store. Tenant ID participates in every ownership key and query. Completed generations and audit events are append-only. State mutation, audit record, and outbox record commit atomically. Idempotency is scoped to tenant, principal, operation, key, and normalized request digest.

## Consequences

Provider-specific Kubernetes and object-store identifiers do not leak into public identity. At-least-once event consumers deduplicate by event ID.

## Verification

`contracts/openapi.yaml`, `database-model.md`, and future API/database tests are normative evidence.
