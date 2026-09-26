package ports

import (
	"context"
	"errors"

	"github.com/bdobrica/ThinkPixelWS/internal/domain"
	"github.com/bdobrica/ThinkPixelWS/internal/domain/shared"
)

var ErrIdempotencyConflict = errors.New("idempotency key was used with a different request")

// WorkspaceCreation is one atomic create mutation. The store must persist the
// Workspace, audit, outbox and completed idempotency result together, or none.
type WorkspaceCreation struct {
	Workspace     domain.Workspace
	Principal     string
	KeyHash       shared.SHA256Digest
	RequestDigest shared.SHA256Digest
	Audit         domain.AuditEvent
	Outbox        domain.OutboxMessage
}

// WorkspaceCreator serializes concurrent retries within tenant/principal/key
// scope. A matching retry returns the original create result without new events.
// This narrow seam does not require unrelated mutation repositories to exist.
type WorkspaceCreator interface {
	CreateWorkspace(context.Context, WorkspaceCreation) (domain.Workspace, error)
}
