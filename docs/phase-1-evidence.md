# Phase 1 evidence

- Date: 2026-09-01
- Scope: ENG-001 through ENG-019
- Source verification: `GOTOOLCHAIN=go1.25.14 make verify`
- Image verification: `docker build --tag thinkpixelws:eng018-clean .`

## Acceptance evidence

Phase 1's exit criterion is a successful verification run from a clean checkout.
ENG-018 exercised that criterion in a native Linux clone after correcting two
checkout-portability defects that the Windows-mounted development tree had
masked: the service-image checker's executable bit and a link to an ignored
local file.

The clean-checkout source gate completed formatting, vet, Staticcheck, CI-policy,
repository-hygiene, OpenAPI validation and drift, service-image policy, unit,
race, vulnerability, license, build, and Phase 0 contract/documentation checks.
The vulnerability scan reported no known vulnerabilities. The separately built
service image produced manifest-list digest
`sha256:7ffbd60b6ae73531895a82c4ee8f4cefcb1b97102a1087fed296f47c9d03df82`.

The Phase 1 closeout reran the aggregate source gate from the repository state
used to publish this record. CI independently runs the same `make verify` source
gate and a separate service-image build.

## Requirement traceability

| TODO range | Evidence |
|---|---|
| ENG-001–003 | pinned Go module and toolchain, repository boundaries, dependency/source/license policy |
| ENG-004–006 | typed configuration, structured redacting logs, Prometheus registry, and OpenTelemetry initialization with unit and race coverage |
| ENG-007 | shared UUIDv7, clock, typed-error, bounded-string, digest, and authenticated-cursor primitives |
| ENG-008–009 | baseline HTTP server and reproducible OpenAPI validation/generation drift checks |
| ENG-010–011 | repository-root Make targets and aggregate format, vet, lint, unit, race, vulnerability, license, and build verification |
| ENG-012–013 | explicit PostgreSQL development/migration workflow and API-backed CLI skeleton |
| ENG-014–015 | hardened non-root service image and pinned, least-privilege CI policy checks |
| ENG-016 | tracked-tree and staged-tree repository-hygiene controls with self-tests |
| ENG-017 | supported-version and compatibility matrix |
| ENG-018 | clean native-Linux checkout `make verify` and Docker image build |
| ENG-019 | this archived Phase 1 evidence record and completion commit |

## Scope and follow-up

This evidence establishes the engineering foundation only. It does not claim
that the Phase 2 domain, persistence, identity, or authorization behavior is
implemented. PostgreSQL was exercised for the explicit empty migration in
ENG-012; real persistence, tenant-isolation, and concurrency tests remain Phase
2 work. Kubernetes/CSI and cross-target portability evidence remain assigned to
later phases.

Tool update notices, generated-code warnings, and dependency assembly-license
notices emitted by the gate were informational and did not bypass a failing
check. The recorded image digest identifies the local clean-checkout build; it
is not a published release artifact or supply-chain attestation.
