# Portable snapshot evaluation

## Decision criteria

Candidates are assessed for correctness/integrity, provider independence, deterministic reconstruction, deduplication, encryption, streaming, partial failure recovery, large-file behavior, ecosystem maturity, and operational simplicity. Correctness and portability are gates; performance cannot compensate for failing either.

| Candidate | Correctness/verification | Portability | Dedup | Operational complexity | Decision |
|---|---|---|---|---|---|
| CSI snapshots/clones | provider-defined | low | provider-defined | low locally | checkpoint only |
| Single tar(+compression) object | digest verifies whole archive | high | none | low | rejected as canonical format |
| Restic repository | mature chunking/encryption | high | strong | external repository/index lifecycle | viable adapter/fallback |
| Kopia repository | mature chunking/encryption/policies | high | strong | external server/repository semantics | viable adapter/fallback |
| WS versioned CAS manifest + immutable blobs | explicit per-file/blob verification | high | strong by digest | moderate, narrow format owned by WS | selected RC baseline |

## Selected format

The RC baseline is a canonical JSON manifest plus immutable SHA-256 addressed blobs in an S3-compatible `PortableStore`. Files are represented by ordered blob spans; small files may be one blob and large files use deterministic content-defined chunking selected during Phase 6 benchmarking. Directory entries contain normalized relative paths, type, portable mode bits, size, and content digest. Ownership, ACLs, xattrs, devices, sockets, and links are excluded from v1 unless explicitly and safely versioned.

The manifest is serialized using RFC 8785 JSON Canonicalization Scheme and hashed with SHA-256 before signing/encryption metadata is attached. Each blob records plaintext digest and length and is encrypted with AEAD using tenant/Workspace/generation/blob context as associated data. Upload is write-once/conditional; completion publishes the manifest only after every blob is verified. Restore verifies schema/version, tenant/Workspace, residency, key context, manifest digest, every blob tag/digest/length, path safety, and final component digest before activation.

## Benchmark protocol

Phase 0 cannot claim storage performance without an implementation. Phase 6 will compare the selected implementation and at least Restic or Kopia using reproducible fixtures: 1 GiB small-file source tree, 20 GiB mixed repository, 100 GiB large-artifact tree, 1% and 20% incremental changes, duplicate components, interrupted upload/restore, and corruption. Record snapshot/restore wall time, throughput, CPU, peak memory, object count, stored bytes/dedup ratio, API requests, and recovery time on two object stores/storage targets.

The selection is architectural, not a fabricated benchmark result. If measured results fail the target envelope or implementation correctness, an ADR supersedes this decision before portable state is released.
