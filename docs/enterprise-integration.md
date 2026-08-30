# Enterprise integration contracts

## ThinkPixelAR WorkspaceBinding

AR requests a Materialization using an AG grant reference and target context. WS returns an opaque binding; AR never needs PVC/object-store internals.

```json
{
  "workspaceId": "01991e0b-7f42-7d68-8c2a-b684d1be7e31",
  "generation": 42,
  "componentAccess": [{"componentId": "01991e0b-8a11-7d68-8c2a-b684d1be7e31", "mode": "read-write"}],
  "materializationId": "01991e0c-1388-7ac1-96a4-b683598714cf",
  "handle": "opaque:provider-bound-value",
  "mountRoot": "/workspace",
  "expiresAt": "2026-08-30T13:00:00Z"
}
```

The handle is audience- and target-bound, non-portable, and non-authorizing. AR owns sandbox creation, attachment, scheduling, termination, and Session/Execution/Attempt state. WS owns content preparation, lease/fence, checkpoint/commit, and release of WS-created storage.

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

## Enterprise blueprint reconciliation

The contracts preserve the platform boundaries in `PLAN.md`: WS owns durable context and immutable provenance/taint; AG owns Run authority; AR owns execution; TG owns governed source/external access and credentials; MP owns qualified software; GR or policy consumers evaluate risk. Provenance and taints are append-only inputs to policy and cannot themselves grant access. No fail-open path is defined.
