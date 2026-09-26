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
not expose this operation yet. AG verification, persisted component scope, public
handle resolution and AR attachment remain to be wired. The existing internal
`materialization.BindingReader` supplies trusted storage-adapter instructions;
it is not this public contract and must not be exposed directly as authorization.
The contract tests use a fixture resolver, not a real AG or storage provider.

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
