package ports

import (
	"context"

	"github.com/bdobrica/ThinkPixelWS/internal/domain"
	"github.com/google/uuid"
)

// WorkspaceRecord includes the current committed head, or zero when absent.
type WorkspaceRecord struct {
	domain.Workspace
	HeadGeneration uint64
}

// WorkspaceReader uses an exclusive UUID keyset boundary within one tenant.
// List returns at most limit records in ascending Workspace ID order.
type WorkspaceReader interface {
	GetWorkspace(context.Context, uuid.UUID, uuid.UUID) (WorkspaceRecord, error)
	ListWorkspaces(context.Context, uuid.UUID, uuid.UUID, int) ([]WorkspaceRecord, error)
}
