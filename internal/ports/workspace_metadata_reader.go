package ports

import (
	"context"
	"errors"

	"github.com/bdobrica/ThinkPixelWS/internal/domain"
	"github.com/google/uuid"
)

var ErrWorkspaceMetadataNotFound = errors.New("workspace metadata not found")

// ComponentRecord describes a stable component, its current source binding,
// and optional classification metadata from the Workspace's committed head.
type ComponentRecord struct {
	domain.WorkspaceComponent
	Source         *domain.SourceBinding
	Classification domain.Classification
	Taints         []string
}

// WorkspaceMetadataReader scopes every lookup to both tenant and Workspace.
// Lists use exclusive keyset boundaries in ascending ID/number order.
type WorkspaceMetadataReader interface {
	GetComponent(context.Context, uuid.UUID, uuid.UUID, uuid.UUID) (ComponentRecord, error)
	ListComponents(context.Context, uuid.UUID, uuid.UUID, uuid.UUID, int) ([]ComponentRecord, error)
	GetGeneration(context.Context, uuid.UUID, uuid.UUID, int) (domain.WorkspaceGeneration, error)
	ListGenerations(context.Context, uuid.UUID, uuid.UUID, int, int) ([]domain.WorkspaceGeneration, error)
}
