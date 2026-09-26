package ports

import (
	"context"

	"github.com/bdobrica/ThinkPixelWS/internal/domain"
	"github.com/bdobrica/ThinkPixelWS/internal/domain/shared"
	"github.com/google/uuid"
)

// GenerationCommit is trusted WS input, not a public request or an authority
// grant. Before calling, authorize under AG governance and capture and verify
// immutable content from this writer outside the metadata transaction.
// Principal, RunID and ExecutionID must come from trusted governance/runtime
// context, not untrusted workspace content. Optional IDs record attribution,
// not authority; WS does not validate them against another component database.
// The manifest must describe that complete capture, not the mutable PVC itself.
type GenerationCommit struct {
	Writer                 MaterializationWriter
	ExpectedHead           uint64
	MaterializationVersion uint64
	GenerationID           uuid.UUID
	ManifestDigest         shared.SHA256Digest
	Durability             domain.GenerationDurability
	Principal              string
	RunID                  *uuid.UUID
	ExecutionID            *uuid.UUID
	RequestID, TraceID     string
}

// GenerationCommitter publishes prepared content from a current writable
// Materialization. Generation, head, audit and outbox commit atomically. It
// neither captures content nor changes the Materialization's base or lifecycle.
// A failed publication must not cause cleanup of content already referenced by
// a generation. No automatic retry is promised after an ambiguous DB result;
// reconcile the stable GenerationID before retrying at a higher layer.
type GenerationCommitter interface {
	Commit(context.Context, GenerationCommit) (domain.WorkspaceGeneration, error)
}
