# ADR-0004: Components, provenance, and bindings

- Status: Accepted
- Date: 2026-08-30

## Decision

A Workspace is multi-component. RC component kinds are `repository`, `directory`, `document-collection`, and `artifact-collection`. Paths are deterministic children of `/workspace`, use a single normalized POSIX segment per component, and may not overlap after Unicode NFC normalization and case folding.

Every imported component generation carries immutable provenance: provider/source identity, resolved source revision, import time, initiating principal and optional Run, derivation edges, classification, source trust, and taints. Classifications are ordered `public < internal < confidential < restricted`; trust values are `trusted-internal`, `authenticated-external`, and `external-untrusted`; taints are additive namespaced labels, including `high-taint`.

Source modes are `snapshot`, `refreshable-snapshot`, and `live-reference`. External, application-profile, environment, and artifact bindings are references, not authority. Authenticated profiles are excluded from ordinary forks and portable snapshots unless an explicit profile grant and policy authorize a separate operation.

## Consequences

Refresh creates a new generation or fork. Source write-back is a separately authorized TG operation. Forks copy immutable provenance, classification, environment/source references, and lineage but not active leases or execution grants.

## Verification

The Workspace Manifest schema and integration contracts encode these vocabularies and restrictions.
