package ports

import (
	"context"
	"errors"

	"github.com/bdobrica/ThinkPixelWS/internal/domain"
	"github.com/google/uuid"
)

var ErrWorkspaceNotFound = errors.New("workspace not found")

// WorkspaceRepository persists Workspace aggregates. Tenant identity is an
// explicit argument so implementations cannot rely on opaque IDs for scope.
type WorkspaceRepository interface {
	Create(context.Context, uuid.UUID, domain.Workspace) error
	Get(context.Context, uuid.UUID, uuid.UUID) (domain.Workspace, error)
}
