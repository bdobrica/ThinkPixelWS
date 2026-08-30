# ADR-0006: Application profiles and environment interoperability

- Status: Accepted
- Date: 2026-08-30

## Decision

The first RC implements the `ProfileProvider` seam only; a portable authenticated browser-profile backend is not RC-required. Profile references do not grant profile access. Materialization requires a short-lived, audience-bound profile grant, encryption, tenant isolation, a named target, and audit of resolve/materialize/checkpoint/release. Profiles are excluded from ordinary content exports and forks.

Environment bindings may reference OCI digests, ThinkPixelMP artifacts, Dev Container definitions, Devfiles, or ThinkPixelAR runtime profiles. WS parses these as untrusted requirements. Only a trusted policy mapper may translate them into runtime configuration. Host paths, privileged mode, host namespaces, devices, arbitrary service accounts, cluster-scoped resources, and unbounded capabilities are denied by default.

## Consequences

ThinkPixelMP references must be immutable digest-qualified for committed generations. Mutable tags may be accepted only as input and resolved before commit. WS does not build or execute environments.

## Verification

Profile/environment schemas and future policy tests ensure references cannot become ambient runtime privilege.
