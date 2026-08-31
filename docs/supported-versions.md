# Supported versions policy

Phase 0 pins the intended first implementation/test baseline. A version becomes production-supported only after its Phase-specific integration suite passes; untested entries are design targets.

| Component | Baseline | Policy |
|---|---|---|
| Go | 1.25.14 | latest security patch in the 1.25 line; N and N-1 after implementation CI proves both |
| PostgreSQL | 17.x | latest minor; PostgreSQL 16 may be added after migration/integration tests |
| Kubernetes | 1.34.x | latest patch; N and N-1 after conformance/integration tests |
| CSI spec | 1.11 | driver must expose required capabilities through Kubernetes objects |
| CSI snapshot API | `snapshot.storage.k8s.io/v1`, external-snapshotter 8.x | optional provider checkpoint; CRD/controller/driver versions tested as a set |
| S3-compatible object store | S3 API with multipart upload, conditional requests, checksums, and server-side encryption | AWS S3 current API is reference; compatible stores require contract tests |
| OPA | 1.x | latest supported minor/security patch; Rego v1 syntax |
| OpenAPI | 3.1.0 / JSON Schema 2020-12 | canonical API/schema dialect |
| ThinkPixelAG/AR/TG/MP | versioned `v1alpha1` contracts | exact compatible versions pinned when implementations exist |

Dependencies are locked by module/image digest in implementation phases. Kubernetes version skew follows upstream policy only where WS CI explicitly tests it. Security end-of-life or critical vulnerabilities can remove a version before the normal window. Capability discovery, not version strings alone, gates CSI and object-store features.

As of Phase 0 no live Kubernetes/CSI environment exists in this repository, so ARC-049 is specified and pinned but its runtime capability test remains an explicit environment-dependent acceptance item in `phase-0-evidence.md`.
