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

IAM-001 implements `ports.TokenVerifier` in `internal/adapters/oidc`. Construct it with an exact HTTPS issuer, the Workspace API audience, and a clock; an optional HTTP client supplies a trusted transport for private CAs. It accepts compact RS256 JWTs only, requires `exp`, checks `nbf` when present, and uses no clock-skew allowance. Successful verification returns issuer claims for subsequent mapping; it grants no Workspace access. Authorization and HTTP API wiring remain separate work.

IAM-002 implements `security.Authenticator`, which calls `ports.TokenVerifier` before mapping identity. Construct it with the pinned-issuer verifier and `security.ClaimMapping{TenantClaim: "tenant_id", PrincipalClaim: "sub"}` (or explicit issuer-specific claim names). Names select exact top-level claims; there are no default names, aliases, nested paths, or fallback values. Both claims must be JSON strings. Tenant IDs must be canonical UUIDv7; principals must be nonempty, at most 256 Unicode characters, without leading/trailing whitespace or control characters. Values are preserved without normalization. Missing or invalid claims, verification failures, and cancellation return a generic authentication error and no identity. Only tenant ID and principal are returned; role/group claims confer no authority. The configured issuer must supply stable principal identifiers in its tenant namespace; changing issuers requires an explicit identity migration. Mapping does not provision tenants or prove tenant existence, membership, or Workspace access.

Discovery must return the configured issuer exactly and an HTTPS `jwks_uri`. Redirects are rejected. Each HTTP request has a maximum five-second timeout and a 1 MiB response limit; tokens are limited to 16 KiB. Only RSA signing keys of 2048–8192 bits with a nonempty, unique `kid` are eligible. Token-supplied key URLs and embedded keys are never used. Keys are cached for five minutes; an unknown `kid` triggers discovery/JWKS refresh at most once per five seconds. Failed refresh cannot use expired keys. Fresh cached keys remain usable during an issuer outage until the local cache expires. Issuers should rotate with new key IDs; replacement under the same ID is discovered at cache expiry. Errors expose no bearer token, claims, or issuer response.

Dependency security review: `github.com/golang-jwt/jwt/v5` v5.3.1 supplies JWT parsing, signature verification, and registered-claim validation under the MIT license, with no additional module dependencies. The adapter explicitly pins RS256, requires expiry/issuer/audience, and exposes no validation-bypass option. Focused tests exercise signature and algorithm rejection, claim boundaries, discovery failures, rotation, cache expiry, and concurrent verification. The dependency's upstream license and attribution must be retained when distributed.

IAM-003 implements `ports.WorkspaceAuthorizer` and `security.AuthorizeWorkspace`. The helper takes authenticated identity, an explicit administrative action, and a target Workspace ID. Create/list use a nil Workspace ID and remain tenant-scoped; view, fork, delete, archive, restore, request-materialization, and read-profile-metadata require a UUIDv7 target. Missing configuration, invalid identity/scope, unknown actions, cancellation, policy errors, and absent explicit allow all return `security.ErrForbidden`. The zero decision denies access. Callers must resolve existing targets within the authenticated tenant and use the same tenant/target for the operation; the helper itself does not fetch Workspaces or prove their ownership. Decisions are evaluated per call, without caching. Administrative permission never substitutes for an AG execution grant or external-tool authority. The OPA adapter (IAM-004) remains pending. The create endpoint checks this helper before persistence, including on idempotent retries. Get resolves within the authenticated tenant before checking view permission. List checks list permission, scopes its database query to that tenant, and omits candidates without view permission. Every page reevaluates policy; cursors authenticate tenant/principal scope and expiry, and confer no authority.

IAM-005 adds explicit `auth.mode=development` ([setup](configuration.md#development-authentication)). Both listeners must bind literal loopback IPs. API requests require a loopback peer and a matching bearer token from a local file; forwarded and identity headers confer no identity. The token is bounded, compared through constant-time SHA-256 digest comparison, and never logged. The fixed UUIDv7 tenant/principal comes only from operator configuration. The local policy permits create/list/view within that identity and denies every other action. API handlers must still perform tenant-scoped ownership lookup and call `httpserver.AuthorizeWorkspace`; identity in context alone is not permission. A startup warning identifies development mode. There is no automatic fallback from OIDC or policy failures, and no AG/external-tool authority is granted.

Development mode is for trusted local machines only; do not expose it through a reverse proxy, tunnel, or shared/untrusted execution environment. Keep the random token outside Workspace content and restart to rotate it. Token possession impersonates the single configured developer. Health/readiness and loopback metrics stay unauthenticated. By default development auth is disabled: any supplied API handler is blocked with 503 until authentication is configured; with no API handler, unknown routes retain their existing 404 behavior. Workspace creation is available with PostgreSQL configured; production OIDC HTTP integration remains pending.

## Observability and redaction

Logs and traces may contain opaque tenant, Workspace, generation, Materialization, Run, Execution, event, request, and provider-operation IDs. They MUST NOT contain content, filenames classified as sensitive, bearer/cookie headers, request/response bodies, source credentials, key material, signed URLs, profile handles, raw binding refs with secrets, or high-cardinality user input. Metrics use bounded status/kind/provider labels only. Audit stores action, actor, target IDs, policy decision/reference, timestamps, and outcome—not secret values. Redaction is recursive and applies before sampling/export.

## Residual risks

Malicious code with authorized write access can corrupt its writable Materialization, consume quota, or write secrets into normal files. Provider/KMS compromise can affect data under that provider. Checkpoints may contain deleted bytes according to backend semantics. These risks require quotas, isolation, encryption, scanning, retention, provider assurance, and documented incident response in later phases.
