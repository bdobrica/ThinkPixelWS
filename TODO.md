# ThinkPixelWS Release-Candidate TODO

This is the chronological implementation checklist for ThinkPixelWS.

Execute the first unchecked item whose dependencies are complete.

An item is checked only after its acceptance evidence passes.

Follow the coding-agent and commit protocol in `PLAN.md`.

Status notation:

- `[ ]` pending
- `[x]` implemented and verified

Completion metadata format:

    — completed YYYY-MM-DD, commit <sha>, evidence: <commands/artifacts>

---

## Phase 0 — Decisions, threats, and contracts

- [x] ARC-001 Create `docs/`, `docs/adr/`, and `docs/contracts/` structure plus ADR template. — completed 2026-08-30, commit 025d12e, evidence: scripts/validate-phase0.sh; docs/phase-0-evidence.md
- [x] ARC-002 Write system-context diagram covering users, WS, PostgreSQL, hot storage, portable storage, profiles, source systems, AG, AR, TG, MP, Kubernetes, and execution environments. — completed 2026-08-30, commit 025d12e, evidence: scripts/validate-phase0.sh; docs/phase-0-evidence.md
- [x] ARC-003 Write trust-boundary diagram distinguishing Workspace metadata, durable content, Materializations, external systems, application profiles, and execution authority. — completed 2026-08-30, commit 025d12e, evidence: scripts/validate-phase0.sh; docs/phase-0-evidence.md
- [x] ARC-004 Write threat model assuming malicious repositories, archives, documents, symlinks, agents, Materializations, source metadata, and compromised execution environments. — completed 2026-08-30, commit 025d12e, evidence: scripts/validate-phase0.sh; docs/phase-0-evidence.md
- [x] ARC-005 Define normative glossary: Workspace, WorkspaceGeneration, WorkspaceComponent, Materialization, checkpoint, portable snapshot, fork, source binding, external binding, profile binding, environment binding. — completed 2026-08-30, commit 025d12e, evidence: scripts/validate-phase0.sh; docs/phase-0-evidence.md
- [x] ARC-006 Record invariant: Workspace != Materialization. — completed 2026-08-30, commit 025d12e, evidence: scripts/validate-phase0.sh; docs/phase-0-evidence.md
- [x] ARC-007 Record invariant: Workspace != AR Session. — completed 2026-08-30, commit 025d12e, evidence: scripts/validate-phase0.sh; docs/phase-0-evidence.md
- [x] ARC-008 Record invariant: WorkspaceBinding != AccessGrant. — completed 2026-08-30, commit 025d12e, evidence: scripts/validate-phase0.sh; docs/phase-0-evidence.md
- [x] ARC-009 Record invariant: Workspace content must not intentionally contain platform execution credentials. — completed 2026-08-30, commit 025d12e, evidence: scripts/validate-phase0.sh; docs/phase-0-evidence.md
- [x] ARC-010 Define persistent vs portable vs roaming semantics. — completed 2026-08-30, commit 025d12e, evidence: scripts/validate-phase0.sh; docs/phase-0-evidence.md
- [x] ARC-011 Define Workspace lifecycle state machine. — completed 2026-08-30, commit 025d12e, evidence: scripts/validate-phase0.sh; docs/phase-0-evidence.md
- [x] ARC-012 Define Materialization lifecycle state machine. — completed 2026-08-30, commit 025d12e, evidence: scripts/validate-phase0.sh; docs/phase-0-evidence.md
- [x] ARC-013 Define immutable WorkspaceGeneration semantics. — completed 2026-08-30, commit 025d12e, evidence: scripts/validate-phase0.sh; docs/phase-0-evidence.md
- [x] ARC-014 Define Workspace head advancement and optimistic-concurrency rules. — completed 2026-08-30, commit 025d12e, evidence: scripts/validate-phase0.sh; docs/phase-0-evidence.md
- [x] ARC-015 Define one-writable-Materialization default and read-only concurrency. — completed 2026-08-30, commit 025d12e, evidence: scripts/validate-phase0.sh; docs/phase-0-evidence.md
- [x] ARC-016 Define writer lease duration/renewal and monotonic fencing-token semantics. — completed 2026-08-30, commit 025d12e, evidence: scripts/validate-phase0.sh; docs/phase-0-evidence.md
- [x] ARC-017 Define stale-writer terminal behavior and commit rejection. — completed 2026-08-30, commit 025d12e, evidence: scripts/validate-phase0.sh; docs/phase-0-evidence.md
- [x] ARC-018 Define Workspace ownership scopes: user, team, service identity, project where supported. — completed 2026-08-30, commit 025d12e, evidence: scripts/validate-phase0.sh; docs/phase-0-evidence.md
- [x] ARC-019 Define tenant and Workspace administrative authorization model. — completed 2026-08-30, commit 025d12e, evidence: scripts/validate-phase0.sh; docs/phase-0-evidence.md
- [x] ARC-020 Define Workspace classification and source trust/taint vocabulary. — completed 2026-08-30, commit 025d12e, evidence: scripts/validate-phase0.sh; docs/phase-0-evidence.md
- [x] ARC-021 Define provenance schema including source, source revision, import actor/Run, derived-from, classification, and taint. — completed 2026-08-30, commit 025d12e, evidence: scripts/validate-phase0.sh; docs/phase-0-evidence.md
- [x] ARC-022 Define WorkspaceComponent kind vocabulary. — completed 2026-08-30, commit 025d12e, evidence: scripts/validate-phase0.sh; docs/phase-0-evidence.md
- [x] ARC-023 Define deterministic component path/mount namespace and collision rules. — completed 2026-08-30, commit 025d12e, evidence: scripts/validate-phase0.sh; docs/phase-0-evidence.md
- [x] ARC-024 Define repository component schema. — completed 2026-08-30, commit 025d12e, evidence: scripts/validate-phase0.sh; docs/phase-0-evidence.md
- [x] ARC-025 Define generic directory component schema. — completed 2026-08-30, commit 025d12e, evidence: scripts/validate-phase0.sh; docs/phase-0-evidence.md
- [x] ARC-026 Define document-collection component schema. — completed 2026-08-30, commit 025d12e, evidence: scripts/validate-phase0.sh; docs/phase-0-evidence.md
- [x] ARC-027 Define ArtifactBinding schema. — completed 2026-08-30, commit 025d12e, evidence: scripts/validate-phase0.sh; docs/phase-0-evidence.md
- [x] ARC-028 Define ExternalBinding schema and modes. — completed 2026-08-30, commit 025d12e, evidence: scripts/validate-phase0.sh; docs/phase-0-evidence.md
- [x] ARC-029 Define ApplicationProfileBinding schema. — completed 2026-08-30, commit 025d12e, evidence: scripts/validate-phase0.sh; docs/phase-0-evidence.md
- [x] ARC-030 Define EnvironmentBinding schema. — completed 2026-08-30, commit 025d12e, evidence: scripts/validate-phase0.sh; docs/phase-0-evidence.md
- [x] ARC-031 Define Workspace Manifest `v1alpha1`. — completed 2026-08-30, commit 025d12e, evidence: scripts/validate-phase0.sh; docs/phase-0-evidence.md
- [x] ARC-032 Define source snapshot, refreshable-snapshot, and live-reference semantics. — completed 2026-08-30, commit 025d12e, evidence: scripts/validate-phase0.sh; docs/phase-0-evidence.md
- [x] ARC-033 Define explicit separation between source import and source write-back. — completed 2026-08-30, commit 025d12e, evidence: scripts/validate-phase0.sh; docs/phase-0-evidence.md
- [x] ARC-034 Define `SourceProvider` interface. — completed 2026-08-30, commit 025d12e, evidence: scripts/validate-phase0.sh; docs/phase-0-evidence.md
- [x] ARC-035 Define `MaterializationProvider`/working-storage interface. — completed 2026-08-30, commit 025d12e, evidence: scripts/validate-phase0.sh; docs/phase-0-evidence.md
- [x] ARC-036 Define provider checkpoint interface. — completed 2026-08-30, commit 025d12e, evidence: scripts/validate-phase0.sh; docs/phase-0-evidence.md
- [x] ARC-037 Define `PortableStore` interface. — completed 2026-08-30, commit 025d12e, evidence: scripts/validate-phase0.sh; docs/phase-0-evidence.md
- [x] ARC-038 Benchmark/research candidate portable snapshot approaches and record decision criteria. — completed 2026-08-30, commit 025d12e, evidence: scripts/validate-phase0.sh; docs/phase-0-evidence.md
- [x] ARC-039 Select initial portable snapshot implementation/format based on correctness, portability, dedup potential, performance, and operational simplicity. — completed 2026-08-30, commit 025d12e, evidence: scripts/validate-phase0.sh; docs/phase-0-evidence.md
- [x] ARC-040 Define PortableSnapshot manifest and integrity rules. — completed 2026-08-30, commit 025d12e, evidence: scripts/validate-phase0.sh; docs/phase-0-evidence.md
- [x] ARC-041 Define snapshot vs checkpoint vs WorkspaceGeneration durability semantics. — completed 2026-08-30, commit 025d12e, evidence: scripts/validate-phase0.sh; docs/phase-0-evidence.md
- [x] ARC-042 Define encryption/key-provider contract for portable state. — completed 2026-08-30, commit 025d12e, evidence: scripts/validate-phase0.sh; docs/phase-0-evidence.md
- [x] ARC-043 Define profile-provider contract and credential-adjacent state rules. — completed 2026-08-30, commit 025d12e, evidence: scripts/validate-phase0.sh; docs/phase-0-evidence.md
- [x] ARC-044 Decide whether browser-profile backend is RC-required or provider-seam-only for first RC. — completed 2026-08-30, commit 025d12e, evidence: scripts/validate-phase0.sh; docs/phase-0-evidence.md
- [x] ARC-045 Define browser-profile materialization authority and audit semantics. — completed 2026-08-30, commit 025d12e, evidence: scripts/validate-phase0.sh; docs/phase-0-evidence.md
- [x] ARC-046 Define environment interoperability policy for Dev Container and Devfile. — completed 2026-08-30, commit 025d12e, evidence: scripts/validate-phase0.sh; docs/phase-0-evidence.md
- [x] ARC-047 Define trusted mapping rule preventing environment metadata from directly granting host/Kubernetes privilege. — completed 2026-08-30, commit 025d12e, evidence: scripts/validate-phase0.sh; docs/phase-0-evidence.md
- [x] ARC-048 Define Kubernetes/CSI working-materialization approach without introducing a custom WS operator. — completed 2026-08-30, commit 025d12e, evidence: scripts/validate-phase0.sh; docs/phase-0-evidence.md
- [x] ARC-049 Pin/test Kubernetes, CSI snapshot API, and selected storage capabilities. — completed 2026-08-30, commit 025d12e, evidence: scripts/validate-phase0.sh; docs/phase-0-evidence.md
- [x] ARC-050 Define Kubernetes Agent Sandbox integration boundary with AR. — completed 2026-08-30, commit 025d12e, evidence: scripts/validate-phase0.sh; docs/phase-0-evidence.md
- [x] ARC-051 Define AR `WorkspaceBinding` contract. — completed 2026-08-30, commit 025d12e, evidence: scripts/validate-phase0.sh; docs/phase-0-evidence.md
- [x] ARC-052 Define AG execution Workspace-access grant contract. — completed 2026-08-30, commit 025d12e, evidence: scripts/validate-phase0.sh; docs/phase-0-evidence.md
- [x] ARC-053 Define component subset and read/write access semantics in AG grant. — completed 2026-08-30, commit 025d12e, evidence: scripts/validate-phase0.sh; docs/phase-0-evidence.md
- [x] ARC-054 Define WS behavior when AG grant is cancelled, expired, or revoked. — completed 2026-08-30, commit 025d12e, evidence: scripts/validate-phase0.sh; docs/phase-0-evidence.md
- [x] ARC-055 Define TG governed source-import contract. — completed 2026-08-30, commit 025d12e, evidence: scripts/validate-phase0.sh; docs/phase-0-evidence.md
- [x] ARC-056 Define external live-binding relationship to TG. — completed 2026-08-30, commit 025d12e, evidence: scripts/validate-phase0.sh; docs/phase-0-evidence.md
- [x] ARC-057 Define MP environment/template artifact reference semantics. — completed 2026-08-30, commit 025d12e, evidence: scripts/validate-phase0.sh; docs/phase-0-evidence.md
- [x] ARC-058 Define data-residency and placement policy contract. — completed 2026-08-30, commit 025d12e, evidence: scripts/validate-phase0.sh; docs/phase-0-evidence.md
- [x] ARC-059 Define locality hints vs scheduling authority. — completed 2026-08-30, commit 025d12e, evidence: scripts/validate-phase0.sh; docs/phase-0-evidence.md
- [x] ARC-060 Define Workspace fork semantics and profile-binding behavior during fork. — completed 2026-08-30, commit 025d12e, evidence: scripts/validate-phase0.sh; docs/phase-0-evidence.md
- [x] ARC-061 Define Workspace archive/restore/delete and retention semantics. — completed 2026-08-30, commit 025d12e, evidence: scripts/validate-phase0.sh; docs/phase-0-evidence.md
- [x] ARC-062 Define legal-hold seam where enterprise policy requires it. — completed 2026-08-30, commit 025d12e, evidence: scripts/validate-phase0.sh; docs/phase-0-evidence.md
- [x] ARC-063 Define secret-persistence prevention and optional scanning hooks. — completed 2026-08-30, commit 025d12e, evidence: scripts/validate-phase0.sh; docs/phase-0-evidence.md
- [x] ARC-064 Define path/archive/symlink security limits. — completed 2026-08-30, commit 025d12e, evidence: scripts/validate-phase0.sh; docs/phase-0-evidence.md
- [x] ARC-065 Define API authentication, RFC 7807, UUIDv7, pagination, idempotency, tracing, request limits, and SSE. — completed 2026-08-30, commit 025d12e, evidence: scripts/validate-phase0.sh; docs/phase-0-evidence.md
- [x] ARC-066 Draft OpenAPI for Workspaces, components, generations, Materializations, checkpoints, commits, forks, snapshots, imports, lifecycle, and events. — completed 2026-08-30, commit 025d12e, evidence: scripts/validate-phase0.sh; docs/phase-0-evidence.md
- [x] ARC-067 Define PostgreSQL schema model, tenant scoping, immutability, leases, audit, outbox, idempotency, and migration strategy. — completed 2026-08-30, commit 025d12e, evidence: scripts/validate-phase0.sh; docs/phase-0-evidence.md
- [x] ARC-068 Define Workspace event vocabulary. — completed 2026-08-30, commit 025d12e, evidence: scripts/validate-phase0.sh; docs/phase-0-evidence.md
- [x] ARC-069 Define observability/redaction contract. — completed 2026-08-30, commit 025d12e, evidence: scripts/validate-phase0.sh; docs/phase-0-evidence.md
- [x] ARC-070 Define target SLOs and RPO/RTO assumptions for hot state, committed generations, portable snapshots, materialization, restore, and roaming. — completed 2026-08-30, commit 025d12e, evidence: scripts/validate-phase0.sh; docs/phase-0-evidence.md
- [x] ARC-071 Define supported-version policy for Go, PostgreSQL, Kubernetes, CSI, object store, OPA, and integration components. — completed 2026-08-30, commit 025d12e, evidence: scripts/validate-phase0.sh; docs/phase-0-evidence.md
- [x] ARC-072 Reconcile Phase 0 with the enterprise-agent blueprint, especially content provenance/taint and governance boundaries. — completed 2026-08-30, commit 025d12e, evidence: scripts/validate-phase0.sh; docs/phase-0-evidence.md
- [x] ARC-073 Validate schemas/OpenAPI/docs and commit Phase 0 evidence. — completed 2026-08-30, commit 025d12e, evidence: scripts/validate-phase0.sh; docs/phase-0-evidence.md

---

## Phase 1 — Engineering foundation

- [x] ENG-001 Initialize Go module using supported pinned Go version. — completed 2026-08-31, commit 4b4ffc0, evidence: `GOTOOLCHAIN=go1.25.14 go mod verify`; `GOTOOLCHAIN=go1.25.14 go test ./...`; `make verify`
- [x] ENG-002 Create repository structure matching domain/app/ports/adapters boundaries. — completed 2026-08-31, commit 130f585, evidence: `GOTOOLCHAIN=go1.25.14 go test ./...`; `make verify`; `git diff --check`
- [x] ENG-003 Add dependency/source/license policy. — completed 2026-08-31, commit e56509d, evidence: `GOTOOLCHAIN=go1.25.14 go mod verify`; `make verify`; `git diff --check`
- [x] ENG-004 Implement typed configuration with file/environment support, validation, safe defaults, and secret references. — completed 2026-08-31, commit fe9630b, evidence: `GOTOOLCHAIN=go1.25.14 go test ./...`; `GOTOOLCHAIN=go1.25.14 go vet ./...`; `make verify`; `git diff --check`
- [x] ENG-005 Implement structured logging with tenant/Workspace/generation/Materialization correlation and recursive secret redaction. — completed 2026-08-31, commit 3e2ad04, evidence: `GOTOOLCHAIN=go1.25.14 go test ./...`; `GOTOOLCHAIN=go1.25.14 go vet ./...`; `GOTOOLCHAIN=go1.25.14 go test -race ./...`; `make verify`; `git diff --check`
- [ ] ENG-006 Implement Prometheus registry and OpenTelemetry initialization.
- [ ] ENG-007 Implement shared UUIDv7, injectable clock, typed errors, bounded strings, digest types, and authenticated cursors.
- [ ] ENG-008 Implement baseline HTTP server with request IDs, W3C tracing, panic recovery, RFC 7807, limits, graceful shutdown, `/livez`, `/readyz`, `/metrics`.
- [ ] ENG-009 Add OpenAPI generation/validation and drift checks.
- [ ] ENG-010 Create repository-root Makefile.
- [ ] ENG-011 Add format/vet/lint/unit/race/vulnerability/license/build verification.
- [ ] ENG-012 Add PostgreSQL development dependency and explicit migration command.
- [ ] ENG-013 Create `thinkpixelwsctl` CLI skeleton using API client.
- [ ] ENG-014 Create hardened non-root service image.
- [ ] ENG-015 Add CI with pinned/least-privilege jobs where practical.
- [ ] ENG-016 Add repository hygiene checks preventing Workspace snapshots, source credentials, browser profiles, kubeconfigs, tokens, keys, and local test data from Git.
- [ ] ENG-017 Start `docs/supported-versions.md`.
- [ ] ENG-018 Verify clean checkout baseline.
- [ ] ENG-019 Publish Phase 1 evidence and commit.

---

## Phase 2 — Durable Workspace domain and metadata

- [ ] DB-001 Add migration framework and tenant schema.
- [ ] DB-002 Add Workspace table/domain/repository.
- [ ] DB-003 Add Workspace lifecycle transitions and optimistic state version.
- [ ] DB-004 Add WorkspaceGeneration table/domain.
- [ ] DB-005 Enforce completed generation immutability.
- [ ] DB-006 Add monotonic per-Workspace generation number.
- [ ] DB-007 Add current-head generation reference.
- [ ] DB-008 Add WorkspaceComponent table/domain.
- [ ] DB-009 Add repository component metadata.
- [ ] DB-010 Add directory/document component metadata.
- [ ] DB-011 Add SourceBinding persistence.
- [ ] DB-012 Add ExternalBinding persistence.
- [ ] DB-013 Add ApplicationProfileBinding persistence.
- [ ] DB-014 Add EnvironmentBinding persistence.
- [ ] DB-015 Add component provenance table/domain.
- [ ] DB-016 Add classification/taint metadata.
- [ ] DB-017 Add WorkspaceFork lineage metadata.
- [ ] DB-018 Add retention/lifecycle policy metadata.
- [ ] DB-019 Add WorkspaceEvent append-only store.
- [ ] DB-020 Add AuditEvent transaction coupling.
- [ ] DB-021 Add IdempotencyRecord.
- [ ] DB-022 Add transactional OutboxMessage.
- [ ] DB-023 Add transaction manager/repository interfaces.
- [ ] IAM-001 Implement OIDC/JWT issuer/audience/algorithm/expiry validation.
- [ ] IAM-002 Implement claim-to-tenant/principal mapping.
- [ ] IAM-003 Implement Workspace administrative authorization port.
- [ ] IAM-004 Implement OPA/Rego reference authorization adapter.
- [ ] IAM-005 Implement explicit safe development auth mode.
- [ ] API-001 Implement create Workspace.
- [ ] API-002 Implement list/get Workspace with tenant-safe pagination.
- [ ] API-003 Implement component/generation read APIs.
- [ ] DB-024 Add real PostgreSQL empty-migration tests.
- [ ] DB-025 Add tenant-isolation tests.
- [ ] DB-026 Add concurrent generation/head-update tests.
- [ ] DB-027 Add immutable-generation mutation-rejection tests.
- [ ] DB-028 Add idempotency/outbox race tests.
- [ ] DB-029 Add property/fuzz tests for Workspace and generation state machines.
- [ ] DB-030 Commit Phase 2 with persistence evidence.

---

## Phase 3 — Kubernetes working Materializations

- [ ] MAT-001 Add Materialization schema/domain/repository.
- [ ] MAT-002 Add Materialization lifecycle state machine.
- [ ] MAT-003 Add Materialization provider-neutral binding metadata.
- [ ] MAT-004 Add writable MaterializationLease schema/domain.
- [ ] MAT-005 Add monotonic Workspace writer fence.
- [ ] MAT-006 Enforce one current writable lease by database constraint/transaction.
- [ ] MAT-007 Allow multiple read-only Materializations.
- [ ] MAT-008 Implement lease renewal/expiry.
- [ ] MAT-009 Prevent stale fence from checkpoint/commit.
- [ ] K8S-001 Implement Kubernetes client/configuration adapter.
- [ ] K8S-002 Implement Kubernetes WorkingStorageProvider.
- [ ] K8S-003 Create PVC/hot storage according to configured profile.
- [ ] K8S-004 Implement deterministic component layout.
- [ ] K8S-005 Implement Materialization prepare.
- [ ] K8S-006 Implement Materialization status.
- [ ] K8S-007 Implement attach/binding result for AR.
- [ ] K8S-008 Implement release without deleting canonical Workspace state.
- [ ] K8S-009 Add CSI provider capability discovery.
- [ ] K8S-010 Add resource/capacity error handling.
- [ ] K8S-011 Add no-hostPath/no-privilege guardrails to reference integration.
- [ ] CHK-001 Add provider-local checkpoint schema.
- [ ] CHK-002 Implement CSI VolumeSnapshot checkpoint where supported.
- [ ] CHK-003 Implement checkpoint fallback/unsupported capability behavior.
- [ ] REC-001 Add Materialization reconciler.
- [ ] REC-002 Recover orphaned PVC/Materialization bindings.
- [ ] REC-003 Recover after WS API/reconciler restart.
- [ ] REC-004 Preserve hot storage after AR sandbox deletion.
- [ ] REC-005 Attach replacement sandbox to existing Materialization where supported.
- [ ] K8S-012 Add disposable-cluster integration tests.
- [ ] K8S-013 Add node/Pod replacement tests.
- [ ] MAT-010 Add stale writer after lease takeover tests.
- [ ] MAT-011 Add concurrent read-only/writable behavior tests.
- [ ] MAT-012 Commit Phase 3 with Kubernetes/storage evidence.

---

## Phase 4 — Source import and multi-repository Workspace

- [ ] SRC-001 Implement SourceProvider port.
- [ ] SRC-002 Add Import job schema/domain/repository.
- [ ] SRC-003 Implement bounded archive source adapter.
- [ ] SRC-004 Implement safe archive extraction into component root.
- [ ] SRC-005 Reject path traversal.
- [ ] SRC-006 Reject absolute paths.
- [ ] SRC-007 Reject/contain symlink/hardlink escape.
- [ ] SRC-008 Enforce compressed/uncompressed/file-count limits.
- [ ] GIT-001 Implement public/anonymous Git source adapter for development/reference use.
- [ ] GIT-002 Resolve exact Git commit before import.
- [ ] GIT-003 Record repository/ref/commit provenance.
- [ ] GIT-004 Make import deterministic against resolved commit.
- [ ] GIT-005 Avoid storing Git credentials in Workspace canonical state.
- [ ] TGS-001 Implement ThinkPixelTG source-provider port/adapter skeleton.
- [ ] TGS-002 Define governed repository snapshot/bundle response.
- [ ] TGS-003 Ensure source-system credential remains outside WS/Workspace content.
- [ ] SRC-009 Implement repository component import.
- [ ] SRC-010 Implement generic directory/document collection import.
- [ ] SRC-011 Implement exact component path collision checks.
- [ ] SRC-012 Implement initial Workspace generation from imports.
- [ ] SRC-013 Implement explicit refreshable-source metadata.
- [ ] SRC-014 Implement explicit refresh operation producing new generation/fork according to conflict policy.
- [ ] SRC-015 Do not implement automatic bidirectional source write-back.
- [ ] PROV-001 Persist immutable source provenance.
- [ ] PROV-002 Propagate classification/taint into component generation state.
- [ ] E2E-001 Create Workspace with frontend/backend/infrastructure repositories.
- [ ] E2E-002 Verify deterministic `/workspace/<component>` layout.
- [ ] E2E-003 Verify exact imported commits/provenance.
- [ ] E2E-004 Verify Workspace Materialization contains all three components.
- [ ] SEC-001 Add malicious repository archive fixtures.
- [ ] SEC-002 Verify imports cannot write outside assigned component roots.
- [ ] SRC-016 Commit Phase 4 with multi-repository evidence.

---

## Phase 5 — Commit, generation advancement, snapshots, and forks

- [ ] GEN-001 Implement commit operation from writable Materialization.
- [ ] GEN-002 Require valid current writer fence.
- [ ] GEN-003 Compare expected Workspace head before commit.
- [ ] GEN-004 Reject stale/conflicting head commit.
- [ ] GEN-005 Create new immutable WorkspaceGeneration transactionally.
- [ ] GEN-006 Advance Workspace head atomically.
- [ ] GEN-007 Record initiating principal/Run/Execution provenance.
- [ ] GEN-008 Record parent generation.
- [ ] GEN-009 Record exact component snapshot/checkpoint refs.
- [ ] GEN-010 Mark Materialization clean relative to committed head where possible.
- [ ] SNP-001 Implement provider-native Workspace snapshot coordination across components.
- [ ] SNP-002 Define behavior if only subset of component snapshots succeeds.
- [ ] SNP-003 Prevent partially successful snapshot from becoming committed generation.
- [ ] FRK-001 Implement Workspace fork API/domain.
- [ ] FRK-002 Create new Workspace identity from exact immutable generation.
- [ ] FRK-003 Use CSI clone/copy-on-write where supported.
- [ ] FRK-004 Provide safe full-copy fallback or explicit unsupported result.
- [ ] FRK-005 Preserve provenance/classification/lineage.
- [ ] FRK-006 Do not automatically clone sensitive application profiles unless policy allows.
- [ ] FRK-007 Verify source bindings are references, not implicit external write privileges.
- [ ] CON-001 Stress concurrent commit attempts.
- [ ] CON-002 Stress expired writer returning after another writer commits.
- [ ] CON-003 Verify no lost head update.
- [ ] FRK-008 Modify fork A and fork B independently and prove isolation.
- [ ] FRK-009 Commit Phase 5 with concurrency/fork evidence.

---

## Phase 6 — Portable snapshots and roaming

- [ ] PORT-001 Implement PortableStore port selected in Phase 0.
- [ ] PORT-002 Implement portable content manifest.
- [ ] PORT-003 Implement content digesting/integrity verification.
- [ ] PORT-004 Implement canonical serialization/versioning.
- [ ] PORT-005 Implement export from committed generation.
- [ ] PORT-006 Implement restore into fresh WorkingStorageProvider target.
- [ ] PORT-007 Verify restored component paths/content hashes.
- [ ] PORT-008 Record portable snapshot metadata in PostgreSQL.
- [ ] PORT-009 Add encryption envelope.
- [ ] KEY-001 Implement KeyProvider interface.
- [ ] KEY-002 Implement development key backend.
- [ ] KEY-003 Add production KMS/HSM integration seam.
- [ ] KEY-004 Ensure raw encryption keys never persist in PostgreSQL/logs.
- [ ] PORT-010 Verify restore fails safely with wrong/unavailable key.
- [ ] PORT-011 Add corruption detection tests.
- [ ] PORT-012 Add interrupted upload/export recovery.
- [ ] PORT-013 Add interrupted restore recovery.
- [ ] PORT-014 Add portable snapshot idempotency.
- [ ] ARCH-001 Implement archive operation releasing hot storage after portable-state guarantee.
- [ ] ARCH-002 Implement restore from archived Workspace.
- [ ] RESID-001 Implement residency/placement policy input.
- [ ] RESID-002 Block portable transfer to prohibited target/region.
- [ ] RESID-003 Audit residency denials.
- [ ] ROAM-001 Restore Workspace generation to fresh storage not referencing original PVC.
- [ ] ROAM-002 Destroy original hot storage.
- [ ] ROAM-003 Verify Workspace remains recoverable solely from canonical portable state.
- [ ] ROAM-004 Benchmark snapshot/export/restore for representative Workspace sizes.
- [ ] ROAM-005 Document RPO/RTO of committed generation vs active dirty state.
- [ ] ROAM-006 Publish `docs/portable-workspace-evidence.md`.
- [ ] ROAM-007 Commit Phase 6 as first true roaming Workspace milestone.

---

## Phase 7 — ThinkPixel-integrated MVP

- [ ] TAG-001 Implement ThinkPixelAG execution-authority verifier port.
- [ ] TAG-002 Add secure WS↔AG service authentication.
- [ ] TAG-003 Validate Workspace ID/generation/component subset against AG grant.
- [ ] TAG-004 Validate read-only vs writable mode against AG grant.
- [ ] TAG-005 Reject Materialization request expanding component set.
- [ ] TAG-006 Reject writable Materialization when grant is read-only.
- [ ] TAG-007 Bind Materialization metadata to AG Run/AR Execution references.
- [ ] TAG-008 Handle AG grant expiry/revocation.
- [ ] TAG-009 Stop lease renewal after authority expiry where applicable.
- [ ] TAG-010 Ensure expired authority cannot commit new generation.
- [ ] TAR-001 Implement AR WorkspaceBinding API/client contract.
- [ ] TAR-002 Return provider-neutral Materialization handle usable by AR adapter.
- [ ] TAR-003 Integrate KAS/Kubernetes volume attachment flow with AR.
- [ ] TAR-004 Verify AR Session close does not delete Workspace.
- [ ] TAR-005 Verify AR Sandbox deletion does not delete Workspace.
- [ ] TAR-006 Verify replacement AR Sandbox can reattach/restore Workspace.
- [ ] TTG-001 Complete ThinkPixelTG governed repository-source adapter.
- [ ] TTG-002 Verify raw SCM credential never enters WS canonical content or AR sandbox.
- [ ] TTG-003 Add governed document-source adapter seam.
- [ ] TTG-004 Define governed export/write-back seam without implementing automatic sync.
- [ ] TMP-001 Add ThinkPixelMP immutable EnvironmentBinding reference support.
- [ ] TMP-002 Verify mutable environment aliases resolve to exact digest before durable binding where integrated.
- [ ] E2E-005 AG admits Run with Workspace subset → AR executes → WS materializes allowed components only.
- [ ] E2E-006 Modify Workspace → checkpoint → commit → new generation.
- [ ] E2E-007 Suspend/destroy AR compute → restore/reattach → next Run continues.
- [ ] E2E-008 Attempt access to ungranted Workspace component and verify denial.
- [ ] E2E-009 Attempt source-system write-back without TG capability and verify impossible.
- [ ] E2E-010 Inspect Workspace and snapshots for platform execution credential residue.
- [ ] MVP-001 Run complete ThinkPixel coding Workspace scenario.
- [ ] MVP-002 Publish `docs/mvp-thinkpixel-evidence.md`.
- [ ] MVP-003 Commit Phase 7 integrated milestone.

---

## Phase 8 — External bindings, environment interoperability, and profiles

- [ ] EXT-001 Implement ExternalBinding domain/API.
- [ ] EXT-002 Support collaboration live-reference binding.
- [ ] EXT-003 Support issue/project-system live-reference binding.
- [ ] EXT-004 Support document-source live-reference binding.
- [ ] EXT-005 Ensure ExternalBinding contains no reusable downstream credentials.
- [ ] EXT-006 Ensure binding membership is exposed as context metadata only.
- [ ] EXT-007 Add AG authorization metadata mapping for live bindings.
- [ ] DEV-001 Implement `devcontainer.json` parser/importer for safe portable metadata.
- [ ] DEV-002 Separate environment requirements from unsafe host/runtime directives.
- [ ] DEV-003 Reject/ignore privileged host configuration according to trusted policy.
- [ ] DEV-004 Implement Devfile parser/importer for relevant project/component/volume metadata.
- [ ] DEV-005 Preserve original environment-definition source/digest.
- [ ] ENV-001 Implement EnvironmentBinding API.
- [ ] ENV-002 Support immutable OCI/MP environment reference.
- [ ] PROF-001 Implement ProfileProvider port.
- [ ] PROF-002 Add profile metadata schema with sensitivity class.
- [ ] PROF-003 Add profile materialization grant.
- [ ] PROF-004 Add profile audit events.
- [ ] PROF-005 Add encrypted ProfileCheckpoint metadata.
- [ ] PROF-006 Ensure profile access is separate from ordinary Workspace read access.
- [ ] PROF-007 Ensure profile is not included in generic portable snapshot unless explicitly configured.
- [ ] PROF-008 Implement controlled reference browser-profile backend if Phase 0 decision includes it in RC.
- [ ] PROF-009 Add browser-profile isolation test between Workspaces/tenants.
- [ ] PROF-010 Add profile stale-grant/revocation test.
- [ ] OFF-001 Define office-work reference Workspace containing document collection + browser profile + collaboration binding.
- [ ] OFF-002 Materialize office Workspace into a compatible execution surface if available.
- [ ] OFF-003 Persist document changes while discarding disposable compute.
- [ ] OFF-004 Verify collaboration/browser binding requires independent authority after rematerialization.
- [ ] EXT-008 Commit Phase 8 with external-context/profile evidence.

---

## Phase 9 — Cross-target roaming, security, resilience, and performance hardening

- [ ] CROSS-001 Configure second independent working-storage target.
- [ ] CROSS-002 Prefer second Kubernetes cluster/storage domain rather than second PVC in the same backend.
- [ ] CROSS-003 Restore portable snapshot into second target.
- [ ] CROSS-004 Materialize same Workspace identity on second target.
- [ ] CROSS-005 Verify exact committed generation content/provenance.
- [ ] CROSS-006 Verify no original hot-storage dependency remains.
- [ ] CROSS-007 Verify write on second target advances same Workspace through controlled commit.
- [ ] CROSS-008 Verify residency policy blocks prohibited cross-region target.
- [ ] CROSS-009 Measure cross-target roaming latency and transferred bytes.
- [ ] SEC-003 Add tenant cross-Workspace enumeration tests.
- [ ] SEC-004 Add storage-handle guessing tests.
- [ ] SEC-005 Add cross-tenant portable snapshot restore attempt.
- [ ] SEC-006 Add path traversal/symlink/hardlink attack matrix.
- [ ] SEC-007 Add component path collision attack.
- [ ] SEC-008 Add attempt to persist platform short-lived credentials.
- [ ] SEC-009 Add optional secret-scanning hook and evidence integration if selected.
- [ ] SEC-010 Verify platform credential mounts/paths are outside Workspace content roots.
- [ ] SEC-011 Add malicious Workspace Manifest requesting privileged host/Kubernetes settings and verify no privilege grant.
- [ ] SEC-012 Add stale Materialization replay after new writer.
- [ ] SEC-013 Add deleted/forked Workspace handle reuse tests.
- [ ] SEC-014 Add profile leakage tests where profile backend implemented.
- [ ] CHAOS-001 Kill WS API during Materialization create.
- [ ] CHAOS-002 Kill worker during checkpoint.
- [ ] CHAOS-003 Kill worker during commit.
- [ ] CHAOS-004 Kill worker during portable snapshot export.
- [ ] CHAOS-005 Kill worker during restore.
- [ ] CHAOS-006 Interrupt PostgreSQL.
- [ ] CHAOS-007 Interrupt object store.
- [ ] CHAOS-008 Interrupt Kubernetes API.
- [ ] CHAOS-009 Lose hot volume/provider.
- [ ] CHAOS-010 Reintroduce stale worker/materialization after recovery.
- [ ] CHAOS-011 Verify no terminal generation corruption across repeated fault injection.
- [ ] CAP-001 Add per-tenant Workspace/materialization/storage limits.
- [ ] CAP-002 Add bounded worker queues/backpressure.
- [ ] CAP-003 Load test Workspace listing/metadata.
- [ ] CAP-004 Load test concurrent Materialization requests.
- [ ] CAP-005 Load test multi-repository commit.
- [ ] CAP-006 Load test snapshot/export/restore.
- [ ] CAP-007 Test large-file/large-repository behavior.
- [ ] CAP-008 Document tested capacity envelope/bottlenecks.
- [ ] HARD-001 Publish cross-target roaming/security evidence.
- [ ] HARD-002 Commit Phase 9.

---

## Phase 10 — Production packaging and operations

- [ ] OPS-001 Finalize reproducible non-root ThinkPixelWS image.
- [ ] OPS-002 Finalize `thinkpixelwsctl`.
- [ ] OPS-003 Create Helm chart for API/workers, migration Job, service account, configuration, secrets, Service, optional ingress.
- [ ] OPS-004 Add least-privilege Kubernetes RBAC.
- [ ] OPS-005 Add NetworkPolicies for PostgreSQL, object store, Kubernetes API, AG/TG/MP, and configured providers.
- [ ] OPS-006 Add hardened pod security context, seccomp, dropped capabilities, read-only root filesystem, bounded temp.
- [ ] OPS-007 Add startup/readiness/liveness probes.
- [ ] OPS-008 Add PDB/topology-spread guidance.
- [ ] OPS-009 Add optional HPA.
- [ ] OPS-010 Add ServiceMonitor/PodMonitor where applicable.
- [ ] OPS-011 Create dashboards for Workspace lifecycle, generations, Materializations, leases, snapshots, portable storage, roaming, imports, profiles, provider errors, PostgreSQL.
- [ ] OPS-012 Define SLO alerts and runbook links.
- [ ] OPS-013 Write installation/configuration runbook.
- [ ] OPS-014 Write Kubernetes/CSI provider runbook.
- [ ] OPS-015 Write portable-store/object-store runbook.
- [ ] OPS-016 Write Workspace recovery/orphan Materialization runbook.
- [ ] OPS-017 Write stale writer/fencing incident runbook.
- [ ] OPS-018 Write snapshot corruption/restore runbook.
- [ ] OPS-019 Write data-residency configuration runbook.
- [ ] OPS-020 Write source-provider/TG integration runbook.
- [ ] OPS-021 Write browser/profile security runbook where applicable.
- [ ] OPS-022 Write archive/retention/delete runbook.
- [ ] OPS-023 Write PostgreSQL migration/backup/restore runbook.
- [ ] OPS-024 Test PostgreSQL backup/restore preserving Workspaces, heads, generations, leases, snapshots, forks, audit, idempotency, outbox.
- [ ] OPS-025 Test portable-store backup/disaster-recovery assumptions.
- [ ] OPS-026 Test fresh cluster install.
- [ ] OPS-027 Test schema/chart upgrade.
- [ ] OPS-028 Test failed upgrade and roll-forward/rollback path.
- [ ] OPS-029 Test rolling restart during active Materializations.
- [ ] OPS-030 Test uninstall with explicit preservation/deletion policy for Workspace data.
- [ ] OPS-031 Run production-like load test.
- [ ] OPS-032 Generate SBOM/vulnerability reports.
- [ ] OPS-033 Add build provenance/signing hooks/checksums.
- [ ] OPS-034 Add release automation.
- [ ] OPS-035 Commit Phase 10 with operations evidence.

---

## Phase 11 — Release-candidate closure

- [ ] RC-001 Freeze OpenAPI and error contracts.
- [ ] RC-002 Freeze Workspace Manifest, WorkspaceGeneration, PortableSnapshot, Materialization, EnvironmentBinding, and profile metadata schemas.
- [ ] RC-003 Freeze provider capability contracts for RC.
- [ ] RC-004 Run generated-artifact/backward-compatibility checks.
- [ ] RC-005 Run `make verify` from clean checkout.
- [ ] RC-006 Archive unit/race/fuzz/PostgreSQL/storage/security/e2e/chaos evidence.
- [ ] RC-007 Confirm Workspace identity is independent of PVC, Pod, sandbox, node, and cluster.
- [ ] RC-008 Confirm completed WorkspaceGeneration cannot mutate.
- [ ] RC-009 Confirm stale writer cannot commit after fence takeover.
- [ ] RC-010 Confirm multiple read-only Materializations do not violate write rules.
- [ ] RC-011 Confirm multi-repository generation captures exact combined state.
- [ ] RC-012 Confirm forked Workspaces diverge independently.
- [ ] RC-013 Confirm original hot storage can be destroyed and Workspace restored from portable canonical state.
- [ ] RC-014 Confirm cross-target roaming proof succeeds.
- [ ] RC-015 Confirm residency policy prevents prohibited roaming.
- [ ] RC-016 Confirm Workspace membership cannot grant AG/TG capability authority.
- [ ] RC-017 Confirm AR Session deletion does not implicitly delete Workspace.
- [ ] RC-018 Confirm source-system credentials remain outside Workspace canonical state in integrated flow.
- [ ] RC-019 Confirm execution credentials are not intentionally included in portable snapshots.
- [ ] RC-020 Confirm malicious path/archive inputs cannot escape component roots.
- [ ] RC-021 Confirm browser/profile state receives separate authority and isolation where implemented.
- [ ] RC-022 Confirm Workspace Manifest cannot grant privileged Kubernetes/runtime configuration.
- [ ] RC-023 Confirm archive/restore/delete lifecycle and retention policy.
- [ ] RC-024 Confirm no unresolved critical/high vulnerability/security finding.
- [ ] RC-025 Confirm no required test is flaky/skipped without documented disposition.
- [ ] RC-026 Confirm SLO/RPO/RTO/capacity envelope documented and measured.
- [ ] RC-027 Exercise install, upgrade, rollback/forward, backup/restore, node loss, storage outage, object-store outage, PostgreSQL outage, rolling restart game days.
- [ ] RC-028 Reconcile every TODO against implementation/tests/docs/commits.
- [ ] RC-029 Update README with product boundary, Workspace model, multi-repo, generations, Materializations, roaming, profiles, ThinkPixel integration, deployment, security, and limitations.
- [ ] RC-030 Create numbered ADRs for all durable decisions.
- [ ] RC-031 Ensure ADRs preserve rejected alternatives and implementation lessons.
- [ ] RC-032 Prepare RC release notes, support matrix, operator checklist, storage compatibility matrix, and artifact inventory.
- [ ] RC-033 Document post-RC scope.
- [ ] RC-034 Remove `PLAN.md` and `TODO.md` only after durable rationale is transferred.
- [ ] RC-035 Run documentation/link validation and `make verify`.
- [ ] RC-036 Commit final documentation transition.
- [ ] RC-037 Build release artifacts from exact commit and verify image/checksum/SBOM/provenance consistency.
- [ ] RC-038 Create/tag RC only after all gates pass.

---

## Deferred / post-RC backlog

- [ ] FUT-001 Add additional WorkingStorageProvider implementations.
- [ ] FUT-002 Add native VM/cloud-workstation disk MaterializationProvider.
- [ ] FUT-003 Add Windows workspace/profile provider.
- [ ] FUT-004 Add production browser-profile provider integrations.
- [ ] FUT-005 Add richer office application profile support.
- [ ] FUT-006 Add automated external source refresh workflows.
- [ ] FUT-007 Add governed bidirectional source synchronization only where semantics are safe.
- [ ] FUT-008 Add Git-aware multi-repository change-set/export model.
- [ ] FUT-009 Add patch-set abstraction across multiple repositories.
- [ ] FUT-010 Add richer Devfile/Dev Container interoperability.
- [ ] FUT-011 Add WorkspaceTemplate support.
- [ ] FUT-012 Distribute WorkspaceTemplates through ThinkPixelMP.
- [ ] FUT-013 Add semantic/lexical Workspace metadata search.
- [ ] FUT-014 Add cross-enterprise Workspace sharing/federation.
- [ ] FUT-015 Add additional portable snapshot/dedup backends.
- [ ] FUT-016 Add incremental cross-region snapshot replication.
- [ ] FUT-017 Add cross-region cache/locality optimizer.
- [ ] FUT-018 Add richer legal-hold/data-governance integrations.
- [ ] FUT-019 Add Workspace content DLP/evidence integration.
- [ ] FUT-020 Add collaborative human/agent handoff UX.
- [ ] FUT-021 Add human-editable Workspace mount/access protocol.
- [ ] FUT-022 Add workspace-level artifact lineage/query API.
- [ ] FUT-023 Add durable workflow integration only if Workspace synchronization/migration orchestration proves to require it.

---

## Progress log

Append one row per completed atomic item or tightly coupled group.

Do not delete historical entries.

Supersede obsolete assumptions with a later entry.

Date | TODO IDs | Commit | Verification evidence | Notes/deviations
--- | --- | --- | --- | ---
YYYY-MM-DD | `ARC-...` | `<sha>` | `<commands/artifacts>` | `<notes>`
