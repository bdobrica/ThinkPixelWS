# Security and threat model

## Assets and assumptions

Assets are tenant metadata, work content, provenance, portable snapshots, profile state, encryption keys, authority tokens, audit history, and availability. Repositories, archives, documents, symlinks, source metadata, agents, tools, Materializations, and execution environments are malicious by default. PostgreSQL, WS control-plane workloads, configured KMS, and policy services are trusted but may fail. Storage/provider responses are authenticated but still structurally validated.

## Threats and required controls

| Threat | Boundary/impact | Required controls | Verification |
|---|---|---|---|
| Archive traversal, absolute path, drive/UNC path | source → durable content | descriptor-relative extraction; normalized containment check; reject absolute/`..`/NUL/drive/UNC | hostile archive fixtures |
| Symlink/hardlink escape or race | content → host/other component | reject links by default; if enabled, validate target beneath component root; no follow during extraction; `openat2`-style beneath/no-symlink semantics where available | link-chain and race tests |
| Zip bomb or huge repository | availability/cost | compressed, expanded, ratio, file-count, per-file, depth, and timeout limits; streaming accounting | boundary and cancellation tests |
| Malicious filenames/metadata | database/filesystem/UI | UTF-8/NFC validation, bounded strings, control-character rejection, output encoding | fuzz/property tests |
| Cross-tenant object reference | confidentiality/integrity | tenant in every key/query; opaque IDs insufficient; ownership check at each adapter | PostgreSQL and object-store isolation tests |
| Stale writer after partition/pause | integrity | server-time lease, monotonic fence, expected-head CAS, provider fence propagation | concurrent takeover tests |
| Compromised sandbox reads control credentials | authority | execution credentials outside Workspace mounts; short-lived audience-bound grants; no service-account token by default | residue and mount tests |
| Workspace binding used as grant | external-system compromise | AG/TG authorization on every operation; deny on unavailable/expired/revoked grant | integration denial tests |
| Poisoned source provenance/classification | downstream policy bypass | provider-attested resolved revision; immutable provenance; classification cannot be downgraded without authorized audited operation | provenance tests |
| Snapshot tampering/rollback/substitution | integrity/confidentiality | canonical manifest digest, blob digest/size, AEAD, tenant/Workspace/key context, format/version validation | corruption/wrong-key tests |
| Secrets written into work content | credential persistence | platform mounts excluded; optional scanner gate; findings audited/redacted; documented residual risk | canary credential scans |
| Profile copied as normal content | account takeover | separate provider/storage/key policy; explicit grant; ordinary fork/export exclusion | profile residue tests |
| Environment metadata requests privilege | cluster/host compromise | trusted allow-list mapping; deny privileged/host namespaces/hostPath/devices/capability escalation | policy tests |
| SSRF through source/object references | network compromise | scheme/provider registry; egress policy; DNS/IP validation in adapters; no arbitrary callback URLs | adapter tests |
| Event/log leakage | confidentiality | field allow-list, recursive redaction, no content/tokens/keys/profile data, bounded labels | telemetry tests |
| Delete bypasses legal hold | compliance | transactional hold check; deny key destruction and physical deletion; immutable audit | lifecycle tests |

## Import limits

Defaults are policy-configurable and may only be raised by administrators:

| Limit | Default |
|---|---:|
| compressed upload/archive | 2 GiB |
| expanded import | 20 GiB |
| expansion ratio | 100:1 |
| files | 250,000 |
| single file | 4 GiB |
| path bytes | 1,024 |
| path depth | 64 |
| symlinks and hardlinks | rejected |
| import wall time | 30 minutes |

Extraction occurs in a new empty component root. Existing destinations, device/FIFO/socket entries, setuid/setgid bits, extended security attributes, ownership metadata, and timestamps outside supported ranges are rejected or normalized. Limits are enforced while streaming, not after extraction.

## Credentials and secret scanning

Platform credentials use out-of-tree tmpfs/projected mounts with paths explicitly excluded from checkpoint/export. WS never serializes token-valued environment variables, Kubernetes secrets, kubeconfigs, cloud credentials, KMS plaintext keys, or TG/AG grants. A `SecretScanner` hook returns policy-neutral findings `(rule, confidence, location digest)`; it never stores the secret. Policy may warn, quarantine, or block portable export. Users can still intentionally place secrets in ordinary files; this residual risk is documented and audited.

## Application profiles

Profiles require a short-lived grant bound to tenant, principal/Run, profile reference, target, actions, audience, and expiry. Resolve, materialize, checkpoint, release, export attempt, denial, and secure deletion are audited. Handles are opaque and target-bound. Encryption keys are separate from Workspace content keys. Browser session state is never included in generic portable snapshots or inherited by fork.

## Authentication and authorization

OIDC validation pins issuer, audience, allowed asymmetric algorithms, expiry/not-before, and tenant/principal claims; key rotation follows issuer metadata with fail-closed cache behavior. Administrative operations call `WorkspaceAuthorizer`. Integrated execution operations additionally validate an AG grant. Cancellation, expiry, or revocation immediately blocks new operations and lease renewal; WS requests release and fences the writer. Existing running code may remain until AR terminates it, but it cannot commit through WS.

## Observability and redaction

Logs and traces may contain opaque tenant, Workspace, generation, Materialization, Run, Execution, event, request, and provider-operation IDs. They MUST NOT contain content, filenames classified as sensitive, bearer/cookie headers, request/response bodies, source credentials, key material, signed URLs, profile handles, raw binding refs with secrets, or high-cardinality user input. Metrics use bounded status/kind/provider labels only. Audit stores action, actor, target IDs, policy decision/reference, timestamps, and outcome—not secret values. Redaction is recursive and applies before sampling/export.

## Residual risks

Malicious code with authorized write access can corrupt its writable Materialization, consume quota, or write secrets into normal files. Provider/KMS compromise can affect data under that provider. Checkpoints may contain deleted bytes according to backend semantics. These risks require quotas, isolation, encryption, scanning, retention, provider assurance, and documented incident response in later phases.
