# Supported versions

This document separates versions exercised by the repository from planned
compatibility targets. A version is **verified** only when the named repository
gate exercises it. A **target** records implementation intent and is not a
support claim. Versions not listed as verified are unsupported unless release
notes for a later release say otherwise.

ThinkPixelWS has not published a release candidate. The matrix below therefore
describes the engineering baseline, not a production support commitment.

## Verified engineering baseline

| Dependency or interface | Version | Verification |
|---|---|---|
| Go language and toolchain | Go 1.25; toolchain 1.25.14 | `.go-version` and `go.mod`; `make verify` runs format, vet, Staticcheck, unit, race, vulnerability, license, and build checks |
| CI runner | Ubuntu 24.04 | `.github/workflows/ci.yml` runs `make verify` and builds the service image |
| Development PostgreSQL | PostgreSQL 17.6 on Alpine 3.21 | `compose.yaml` pins the image by tag and digest; `make postgres-up` and `make migrate` exercise the development migration path |
| Migration tool | migrate/migrate 4.19.1 | `compose.yaml` pins the image by tag and digest |
| Service build image | Go 1.25.14 on Debian trixie | `Dockerfile` pins the image by tag and digest; CI builds the image |
| Service runtime image | distroless static Debian 13, non-root | `Dockerfile` pins the image by digest; CI builds the image and `check-service-image` verifies its static hardening rules |
| Public API description | OpenAPI 3.1.0 | `make check-openapi` validates the contract and checks generated Go code for drift |
| JSON Schema contracts | Draft 2020-12 | `make validate-phase0` validates the Workspace manifest and portable-snapshot schemas |

These claims cover repository build and development workflows only. They do
not yet qualify PostgreSQL for production workloads, an operating-system
distribution as a deployment platform, or any Kubernetes, CSI, object-store,
policy-engine, or ThinkPixel cross-component integration.

Python 3, Node.js/npm, Git, Docker, and Docker Compose are developer tools used
by documented commands. Minimum supported versions will be recorded after the
clean-checkout and release qualification environments exercise explicit
versions. Until then, they are prerequisites rather than compatibility claims.

## Planned runtime compatibility

| Component | Initial target | Qualification required before support |
|---|---|---|
| PostgreSQL | 17.x; evaluate 16.x | migration and repository integration suites for every listed major version |
| Kubernetes | 1.34.x; then N and N-1 | conformance, lifecycle, security, and version-skew suites on every listed minor version |
| CSI | CSI spec 1.11 | startup and per-operation capability discovery with a qualified driver/storage-class matrix |
| CSI snapshot API | `snapshot.storage.k8s.io/v1` with external-snapshotter 8.x | CRD, controller, driver, snapshot, restore, clone, and failure-path tests as a versioned set |
| S3-compatible object store | multipart upload, conditional requests, checksums, and server-side encryption | portable-store contract, corruption, retry, and roaming tests for every named implementation |
| OPA | 1.x with Rego v1 syntax | policy decision, failure-mode, and upgrade compatibility tests |
| ThinkPixel integrations | versioned `v1alpha1` AG, AR, TG, and MP wire contracts | contract and end-to-end tests against exact component versions |

Capability discovery remains authoritative for CSI and object-store operations;
a matching version string alone does not enable an optional feature. Kubernetes
version skew follows upstream policy only where ThinkPixelWS CI explicitly
tests the combination.

## Version lifecycle

- Release notes and evidence MUST identify the exact versions used to qualify a
  release. Container dependencies MUST remain pinned by digest where supported.
- Adding or upgrading a verified version requires the corresponding focused
  test and the aggregate `make verify` gate. Integration targets require their
  phase-specific suites before moving into the verified matrix.
- A listed version may be removed early when it reaches upstream end of life,
  cannot receive necessary security fixes, or has a critical incompatible
  vulnerability. The removal and migration guidance belong in release notes.
- Public wire and persisted-format compatibility is governed by the versioned
  contracts and accepted ADRs, not by this matrix. Changing those contracts
  requires their compatibility, documentation, and test updates.

The Kubernetes/CSI rules are defined in
[`operations.md`](operations.md#kubernetescsi-capability-contract), dependency
pinning and exceptions in [`dependency-policy.md`](dependency-policy.md), and
current environment-dependent gaps in
[`phase-0-evidence.md`](phase-0-evidence.md).
