package ports

import (
	"context"
	"errors"
	"time"

	"github.com/bdobrica/ThinkPixelWS/internal/domain"
	"github.com/google/uuid"
)

var (
	ErrWorkspaceNotFound      = errors.New("workspace not found")
	ErrWorkspaceStateConflict = errors.New("workspace state conflict")
)

// WorkspaceRepository persists Workspace aggregates. Tenant identity is an
// explicit argument so implementations cannot rely on opaque IDs for scope.
type WorkspaceRepository interface {
	// AdvanceWriterFence atomically increments and returns the Workspace fence.
	// It does not acquire a lease or grant authority. Acquisition must call it
	// in the same transaction as lease insertion; rollback discards the token.
	// Tokens must not be published before that transaction commits.
	AdvanceWriterFence(context.Context, uuid.UUID, uuid.UUID) (uint64, error)
	Create(context.Context, uuid.UUID, domain.Workspace) error
	Get(context.Context, uuid.UUID, uuid.UUID) (domain.Workspace, error)
	TransitionState(context.Context, uuid.UUID, uuid.UUID, domain.WorkspaceState, domain.WorkspaceState, uint64, time.Time) error
}
