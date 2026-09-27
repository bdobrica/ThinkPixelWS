package ports

import (
	"context"
	"time"

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
	// AuthorityExpiresAt is required, freshly verified by trusted authorization
	// after capture. Never accept a caller-supplied deadline. Explicit local
	// authorization must also supply a finite deadline.
	AuthorityExpiresAt     time.Time
	Writer                 MaterializationWriter
	ExpectedHead           uint64
	MaterializationVersion uint64
	// MarkClean asserts that trusted orchestration has stopped all writes before
	// capture and will keep them stopped until it leaves CHECKPOINTING after
	// publication. A lifecycle label or a valid lease alone does not prove this.
	// False leaves cleanliness unknown, including after a previously clean commit.
	MarkClean      bool
	GenerationID   uuid.UUID
	ManifestDigest shared.SHA256Digest
	// ComponentReferences covers every component in the captured Workspace.
	ComponentReferences []domain.GenerationComponentReference
	Durability          domain.GenerationDurability
	Principal           string
	RunID               *uuid.UUID
	ExecutionID         *uuid.UUID
	RequestID, TraceID  string
}

// GenerationCommitter publishes prepared content from a current writable
// Materialization. Generation, head, audit and outbox commit atomically. It
// neither captures content nor changes the Materialization's base or lifecycle.
// MarkClean records the new generation on a CHECKPOINTING Materialization in
// that same transaction and increments its state version. Clearing an existing
// marker also increments the version. Reload before the next lifecycle mutation.
// A failed publication must not cause cleanup of content already referenced by
// a generation. No automatic retry is promised after an ambiguous DB result;
// reconcile the stable GenerationID before retrying at a higher layer.
type GenerationCommitter interface {
	Commit(context.Context, GenerationCommit) (domain.WorkspaceGeneration, error)
}
