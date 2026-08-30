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
