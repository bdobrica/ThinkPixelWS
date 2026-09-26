# Enterprise integration contracts

## ThinkPixelAR WorkspaceBinding

The published [OpenAPI contract](contracts/openapi.yaml) defines
`POST /v1/materializations/{materialization_id}/binding` (`resolveWorkspaceBinding`)
for an existing READY/ACTIVE Materialization. AR supplies `workspaceId`, exact
`generation`, `targetId`, `componentAccess`, and `executionGrant` in the JSON body.
The grant stays out of URLs. This read-only resolution does not allocate storage,
renew a lease, or attach a sandbox, so it needs no idempotency key.

The generated Go client in `api/openapi` exposes `ResolveWorkspaceBinding` with
typed request/response models and RFC 7807 errors. AR callers must configure an
HTTP transport carrying their service bearer authentication (the generator does
not implement OIDC), use request deadlines, and avoid logging grant bodies.

The response contract is:

```json
{
  "workspaceId": "01991e0b-7f42-7d68-8c2a-b684d1be7e31",
  "generation": 42,
  "componentAccess": [{"componentId": "01991e0b-8a11-7d68-8c2a-b684d1be7e31", "mode": "read-write"}],
  "materializationId": "01991e0c-1388-7ac1-96a4-b683598714cf",
  "handle": "opaque:provider-bound-value",
  "mountRoot": "/workspace",
  "targetId": "homelab",
  "audience": "thinkpixelar",
  "expiresAt": "2026-08-30T13:00:00Z"
}
```

The handle is audience- and target-bound, non-portable, and non-authorizing. AR owns sandbox creation, attachment, scheduling, termination, and Session/Execution/Attempt state. WS owns content preparation, lease/fence, checkpoint/commit, and release of WS-created storage.

Resolution must scope lookup to the authenticated tenant, validate current AG
authority, and require an exact match of Workspace, generation, target and component
modes. Duplicate component IDs are invalid. A requested component subset or mixed
modes must be rejected unless the attachment mechanism enforces that scope; a PVC
mount of the entire Workspace does not enforce component restrictions. Responses
must use `Cache-Control: no-store`. `expiresAt` is bounded by grant expiry and,
for writers, lease expiry. AR still checks current authority, lease/fence, target,
audience, expiry and exclusive attachment at use time. The handle grants nothing.
Missing/cross-tenant identity returns 404, denied authority 403, incompatible state
or scope 409, and unavailable verification/provider observation 503.

**Implementation status:** TAR-001 provides the wire contract, generated Go client
and server interface, and client/server contract tests. The running WS service does
not expose this operation yet. AG verification, persisted component scope, HTTP
handle resolution and AR service composition remain to be wired. The existing internal
`materialization.BindingReader` supplies trusted storage-adapter instructions;
it is not this public contract and must not be exposed directly as authorization.
The contract tests use a fixture resolver, not a real AG or storage provider.

TAR-002 adds a provider-neutral `Binding.Handle` to the internal binding reader.
Trusted integration code calls `BindingReader.ResolveHandle(ctx, tenant, materializationID,
target, audience, handle)` with independently authorized tenant/Materialization/component
scope (the ID comes from that authorization, not the handle) and runtime-configured target and audience (`thinkpixelar`). It reloads the
record and observes the provider, rejects changed records and replacement PVCs,
and returns the current adapter instructions. It does not allocate, attach, renew,
or release storage. Handles survive service restart without an in-memory registry;
a state-version change (including READY to ACTIVE) requires a fresh handle.

The `ws-mat-v1` reference binds tenant, Materialization, Workspace, base generation,
state version, target, access mode, provider identity and AR audience. Consumers
keep the entire value opaque. It contains no PVC name, path, credentials or grant.
Its digest is an identity check, **not a signature or authorization**; constructing
or possessing a handle must never bypass authorization. Binding expiry and current
grant/lease checks remain separate requirements, including at use time.

AR adapters can import `github.com/bdobrica/ThinkPixelWS/api/storagebinding`
without importing WS internals. Decode the resolved `Storage` as `Binding`, reject
unsupported `Kind` values, and decode `kubernetes-pvc-v1` references as `PVCBinding`.
That reference pins namespace, claim name and UID, `/workspace`, and read-only
intent. AR must check the live UID before use, enforce the sandbox namespace and
read-only volume/mount settings, and enforce exclusive writer attachment. These
whole-PVC instructions do not enforce component subsets or mixed component modes.
The application/provider tests exercise neutral-handle resolution and decoding;
no live AR attachment or AG authorization is claimed by TAR-002.

### KAS attachment adapter (TAR-003)

ThinkPixelAR commit `b508aec` adds
`internal/adapters/workspace/kubernetes.WSVolumeResolver`, which
consumes the JSON encoding of the resolved `storagebinding.Binding`. Compose it
with AR's existing `AttachedBlueprintResolver`. A trusted lookup receives the
current AR attachment reservation and must independently verify AG authority,
WS handle scope/expiry and writer lease/fence on every use. The running WS HTTP
service does not supply that lookup yet; never substitute a permissive callback.

The bridge requires the resolved PVC namespace/name/UID to match the already
reserved Workspace volume, preserves the separate vendor-state PVC, and checks
mount root and explicit access mode. AR then verifies live PVC UID, Bound/filesystem
state, capacities, access mode and independently qualified storage properties
before rendering existing claims into the KAS Sandbox. The current coding template
rejects read-only bindings; it does not silently upgrade them. Neither resolution
nor Sandbox creation allocates, owns or deletes WS storage. Claim replacement
between validation and mount must be prevented by trusted cluster ownership because
Pod volume references use names. RWO is not a writer fence.

AR's focused adapter tests exercise WS descriptors through KAS Sandbox creation,
replay without duplicate creation, and fail-closed verification on replay using
local HTTP fixtures. They do not exercise live AG, a WS HTTP endpoint, a database
reservation or the homelab. TAR-003 implements the attachment adapter composition;
the authorized endpoint, durable reservation mapping and running service composition
remain pending. Reproduce the adapter checks from the ThinkPixelAR checkout:

```sh
go test ./internal/adapters/workspace/kubernetes ./internal/adapters/sandbox/agentsandbox
go vet ./internal/adapters/workspace/kubernetes ./internal/adapters/sandbox/agentsandbox
```

## ThinkPixelAG execution grant

The signed/introspected grant MUST contain issuer, audience `thinkpixelws`, grant ID, tenant, principal, Run and optional Execution, Workspace ID, optional exact generation, component allow-list, per-component `read-only`/`read-write` mode, permitted actions, classification ceiling, residency constraints, issued/not-before/expiry times, and revocation/cancellation semantics. It MUST NOT contain downstream credentials.

WS intersects rather than unions permissions: requested components must be a subset; requested mode cannot exceed any per-component mode; Workspace-wide write does not infer external-binding use. A missing component is denied. A read-only grant cannot acquire/renew a writer lease, checkpoint as authoritative, or commit.

On cancellation, expiry, or revocation, WS denies new operations, stops lease renewal, fences writable Materializations, emits an audit/event, requests AR release/termination where configured, and never commits queued work. Authority-service uncertainty fails closed for new privilege and renewal.

## ThinkPixelTG governed source import

WS sends tenant, requesting principal/Run, logical source reference, requested revision, component destination, limits, classification context, and callback audience. TG returns an expiring single-use download handle or streams a bounded snapshot plus:

- provider/source identity;
- exact immutable resolved revision;
- media type, byte count, and SHA-256 digest;
- provider attestation/request ID;
- source classification/trust/taints;
- expiry.

WS validates all metadata and content independently. TG retains source-system credentials. Import is read-only and cannot be replayed as write authority. Export/push/PR/comment is a different TG capability with a separate AG decision and never occurs implicitly after commit.

Live external bindings remain references. At use time, the runtime presents a current AG capability to TG; WS membership supplies context only.

## ThinkPixelMP environment references

Committed generations reference qualified artifacts as `mp://<catalog>/<artifact>@sha256:<digest>` with artifact kind, platform/architecture, qualification policy/version, and optional compatibility requirements. Mutable input tags are resolved by MP before commit. WS stores the immutable reference and metadata, not package bytes or registry credentials. AR resolves/materializes the environment under its own authority and trusted runtime-profile mapping.

## ThinkPixelMEM provenance references

WS remains authoritative for source files, Workspace generations, and their provenance. When MEM records a learned claim derived from Workspace content, WS provides stable references to the supporting Workspace, immutable generation, component, and content digest where available. MEM owns the learned claim and its memory lifecycle; it does not copy that responsibility into WS or become authoritative for Workspace source state.

A provenance reference is evidence, not authority. It does not grant MEM, a Run, or a later consumer access to the referenced Workspace or component. Access requires a current authorization decision through the configured WS authorizer and, for governed execution, AG.

## ThinkPixelGR ingestion evaluation

Imported content may be evaluated by GR through an optional, replaceable policy adapter. WS supplies bounded content or immutable content references plus tenant, classification, provenance, taint, and evaluation-profile context according to deployment policy. GR returns findings or a decision for WS to enforce; a GR result cannot grant Workspace access, expand a Run grant, remove provenance, or silently lower classification or taint.

WS remains responsible for safe extraction, structural validation, and enforcing the configured import outcome. Deployments without GR must retain those baseline controls and use an explicit local policy rather than treating the missing integration as authorization.

## Enterprise blueprint reconciliation

The contracts preserve the platform boundaries in `PLAN.md`: WS owns durable context and immutable provenance/taint; AG owns Run authority; AR owns execution; TG owns governed source/external access and credentials; MP owns qualified software; MEM owns learned claims while referencing WS evidence; GR or another configured policy adapter evaluates content risk. Provenance, memory, taints, and guardrail results are inputs to policy and cannot themselves grant access. No fail-open path is defined.
