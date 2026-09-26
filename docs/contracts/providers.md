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
