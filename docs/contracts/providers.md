# Provider port contracts

Signatures are language-neutral Go-shaped contracts; implementation types remain in the domain/application layers and provider details remain opaque.

```go
type SourceProvider interface {
    Resolve(ctx context.Context, spec SourceSpec) (ResolvedSource, error)
    Import(ctx context.Context, source ResolvedSource, target ImportTarget, limits ImportLimits) (ImportResult, error)
    Refresh(ctx context.Context, binding SourceBinding) (RefreshPlan, error)
}

type MaterializationProvider interface {
    Capabilities(ctx context.Context, target TargetContext) (MaterializationCapabilities, error)
    Prepare(ctx context.Context, req MaterializationRequest) (MaterializationHandle, error)
    Status(ctx context.Context, handle MaterializationHandle) (MaterializationStatus, error)
    Restore(ctx context.Context, req RestoreRequest) (MaterializationHandle, error)
    Release(ctx context.Context, handle MaterializationHandle) error
}

type CheckpointProvider interface {
    Capabilities(ctx context.Context, handle MaterializationHandle) (CheckpointCapabilities, error)
    Checkpoint(ctx context.Context, handle MaterializationHandle, fence uint64) (ProviderCheckpoint, error)
    RestoreCheckpoint(ctx context.Context, checkpoint ProviderCheckpoint, target TargetContext) (MaterializationHandle, error)
    DeleteCheckpoint(ctx context.Context, checkpoint ProviderCheckpoint) error
}

type PortableStore interface {
    PutBlob(ctx context.Context, key ContentDigest, body io.Reader, size int64, opts PutOptions) (BlobReceipt, error)
    HeadBlob(ctx context.Context, key ContentDigest) (BlobMetadata, error)
    GetBlob(ctx context.Context, key ContentDigest, byteRange *ByteRange) (io.ReadCloser, BlobMetadata, error)
    PutManifest(ctx context.Context, id SnapshotID, canonical []byte, condition PutCondition) (ManifestReceipt, error)
    GetManifest(ctx context.Context, id SnapshotID) ([]byte, ManifestMetadata, error)
    DeleteSnapshot(ctx context.Context, id SnapshotID, hold LegalHoldContext) error
}

type KeyProvider interface {
    GenerateDataKey(ctx context.Context, keyContext KeyContext) (PlaintextDataKey, WrappedDataKey, error)
    UnwrapDataKey(ctx context.Context, wrapped WrappedDataKey, keyContext KeyContext) (PlaintextDataKey, error)
    RewrapDataKey(ctx context.Context, wrapped WrappedDataKey, from, to KeyContext) (WrappedDataKey, error)
}

type ProfileProvider interface {
    Resolve(ctx context.Context, ref ProfileRef) (ProfileMetadata, error)
    Materialize(ctx context.Context, ref ProfileRef, grant ProfileGrant, target TargetContext) (ProfileHandle, error)
    Checkpoint(ctx context.Context, handle ProfileHandle, grant ProfileGrant) (ProfileCheckpoint, error)
    Release(ctx context.Context, handle ProfileHandle) error
}
```

## Cross-cutting rules

- Calls are tenant-scoped, context-cancellable, idempotent where a request key is supplied, and emit safe correlation IDs.
- Handles are opaque, bounded strings and cannot be used as public Workspace identity or authority.
- Provider errors distinguish invalid, unauthorized, conflict, unsupported capability, capacity, unavailable, corrupt, and internal outcomes.
- Providers never receive downstream credentials through serializable domain objects. Trusted adapters obtain short-lived credentials out of band.
- `SourceProvider.Resolve` returns an immutable revision and attested metadata. `Import` cannot write outside the pre-opened target.
- A checkpoint fence lower than the current Workspace fence is rejected before and by any capable provider.
- `PortableStore` publishes a manifest only after referenced immutable blobs exist and verify. Deletes honor retention/legal hold.
- Plaintext data keys are zeroizable, process-local, never logged/serialized, and have a bounded lifetime. Wrapped keys carry provider/key/version references only.
- Profile handles and profile encryption domains are separate from ordinary Workspace content.

## Storage lifetime after sandbox deletion

WS creates hot-storage PVCs independently, without Kubernetes owner references.
AR consumes the existing claim by name; it must not adopt the claim, add a
Sandbox/Pod owner reference, put it in Sandbox `volumeClaimTemplates`, or include
it in execution cleanup. WS rejects claims with owner references rather than
silently taking ownership. The namespace containing durable claims must also
outlive execution: deleting a per-sandbox namespace deletes its PVCs regardless
of owner references.

Deleting a Sandbox or its Pods does not request WS release, change Materialization
identity, or discard uncommitted files. Hot storage remains until an explicitly
authorized WS release under the rules below. A StorageClass `Delete` reclaim
policy applies when the PVC is deleted, not when its consuming Pod disappears;
changing it to `Retain` is not required for sandbox survival.

Survival is not renewed execution authority. Lease expiry/fencing still applies,
and replacement execution requires fresh authorization and attachment checks.
Node-local storage survives sandbox deletion only while its node/disk remains
available. The opt-in [homelab test](../homelab-storage.md#sandbox-deletion-survival-test)
checks the real Sandbox controller, PVC/PV identity and file readback. The
[replacement test](../homelab-storage.md#replacement-sandbox-attachment-test) also
checks writable continuation through the existing binding. AR service integration
remains separate work.

## Materialization restart recovery

The internal `materialization.Recoverer.Recover` takes an authorized tenant-scoped
ID and reloads persisted intent. REQUESTED/PREPARING resumes the existing prepare
path; RELEASING resumes release. Allocation interrupted before handle persistence
reuses the deterministic, ownership-checked PVC. A persisted handle must still
match its PVC UID; missing/replaced storage is never silently reconstructed.
Partial preparation retries verified restore under exclusive mounted access.
Deletion completes only after confirmed absence, including when deletion finished
before the previous process persisted RELEASED.

READY/ACTIVE and terminal records return unchanged without provider I/O: a restart
never restores over working edits, releases continuation storage, revives a fenced
writer, or resets lease/fence authority. This is not storage health verification;
use the status/binding readers for observations and current attachment checks.
CHECKPOINTING returns a state conflict with the unchanged record, requiring
checkpoint recovery rather than an assumed successful checkpoint.

Recovery inherits prepare/release authorization, detachment, retention and
independently committed audit/outbox requirements. Callers must re-establish these
preconditions after restart; persisted lifecycle is not an authorization grant.
Pending work retains its state for another retry, and provider/persistence errors
remain visible. This is a per-record application entry point, not an automatic
startup scan or background loop. Tests reconstruct services, repository objects
from serialized records, and HTTP clients while retaining the external PVC fixture
and mounted files; they do not qualify real process/PostgreSQL restarts.

## Materialization release

The internal `materialization.Releaser` accepts an authorized, tenant-scoped ID.
READY or ACTIVE transitions to RELEASING with a version check before provider
deletion. Retries resume RELEASING; only confirmed storage absence permits
RELEASED. A pending deletion returns RELEASING without an error. A RELEASED retry
does no provider I/O. Missing bindings, replacement PVCs, and concurrent lifecycle
changes fail explicitly; FAILED/FENCED records are not revived for cleanup.
Kubernetes deletion uses the persisted PVC identity and resource-version
preconditions, and never removes finalizers to force completion.

Release deletes disposable hot storage only. It retains the Materialization
record and its generation/binding references and has no dependency on canonical
Workspace, generation, snapshot, blob, or key deletion. Uncommitted hot changes
are not preserved by this operation. Callers must authorize their disposal or
durably checkpoint/commit them first, detach execution and prevent reattachment,
and enforce retention and lease policy. Sandbox replacement alone is not a
reason to release a volume needed for continuation. Archive additionally requires
a verified portable snapshot under ADR-0005.

Repository mutations must commit independently with required audit/outbox;
provider calls must not run inside a database transaction. This coordinator does
not retire writer leases or implement runtime detachment, HTTP/process wiring,
or terminal/orphan reconciliation. Those remain caller/integration duties.

## AR storage binding result

The internal `materialization.BindingReader` resolves an already-authorized
Materialization into a serializable storage description. The envelope contains
`tenantId`, `workspaceId`, `generation` (base generation), `materializationId`,
`stateVersion`, `targetId`, `accessMode`, and `storage`. Storage contains the opaque
`handle`, versioned `kind`, and provider-specific `reference`. No credentials are
included. AR selects a trusted adapter by kind and must reject unknown kinds.
This is not yet an HTTP endpoint or an implemented AR integration.

For `kind: kubernetes-pvc-v1`, reference fields are:

| Field | Meaning |
| --- | --- |
| `namespace` | Configured namespace of the existing PVC; execution must use this namespace. |
| `claimName` | Existing PVC name, never a request to create storage. |
| `claimUid` | Expected PVC UID, checked against the persisted opaque handle. |
| `mountPath` | `/workspace`, containing the prepared component layout. |
| `readOnly` | True for read-only Materializations; enforce on the Pod PVC volume source and every container mount. |

Only READY or ACTIVE Materializations with existing bound filesystem storage
produce a result. The reader rechecks persisted metadata after provider I/O;
concurrent changes fail with a state conflict and require a fresh read. Resolution
is repeatable and does not mutate lifecycle, create Pods, or allocate/release PVCs.

The result is storage metadata, not a capability or current lease proof. Before
attachment, the trusted runtime must validate current AG authority, exact target
and scope, and current writer lease/fence for writable access. It must detach old
execution before replacement attachment and prevent overlapping writers. Whole
volume mounting is valid only when the grant covers its entire component scope;
this result does not implement component filtering. Kubernetes RWO is not writer
fencing. A consumer must recheck UID and prevent PVC replacement during attachment:
Pod PVC references identify claims by name and do not offer a UID precondition.
Treat a changed UID as conflict, never silently adopt the replacement. Lifecycle,
lease and provider checks are observations, not a distributed atomic attachment
transaction. Revocation and stale execution termination remain runtime duties.

## Replacement attachment to existing hot storage

A replacement Sandbox uses the same READY/ACTIVE Materialization and resolves a
fresh binding through the existing binding reader. There is no WS allocation,
restore, release, or lifecycle reset for this handoff: restoring the base generation
would overwrite uncommitted work. Missing/replaced storage and FENCED records
remain errors; reattachment cannot revive an expired writer.

The trusted runtime must first stop old execution and confirm all its consuming
Pods have terminated and storage can be safely remounted. It then revalidates AG
scope and the current writer lease/fence, resolves the binding, checks the PVC UID,
and creates replacement execution referencing that existing claim. It must prevent
concurrent attachment and PVC replacement throughout the handoff. These are runtime
preconditions, not additional authority supplied by a binding. If the lease expires
during recovery, stop and follow fenced recovery policy instead of reusing it.

The live test demonstrates sequential attachment on a healthy cluster after normal
foreground deletion; absence from the API after force deletion or loss of contact
with a node is not proof that an old writer has stopped. Node-local storage pins
replacement scheduling to the original storage node. Cross-node recovery after
node/disk loss requires a compatible shared storage backend or restore to a new
Materialization; it is not supplied by local-path attachment.
