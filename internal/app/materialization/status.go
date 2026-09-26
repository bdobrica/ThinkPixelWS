package materialization

import (
	"context"
	"errors"

	"github.com/bdobrica/ThinkPixelWS/internal/domain"
	"github.com/bdobrica/ThinkPixelWS/internal/ports"
	"github.com/google/uuid"
)

// Status preserves the authoritative lifecycle separately from observed storage.
// Storage is nil when no binding exists or observation fails (see returned error).
// Neither lifecycle nor a bound volume proves current lease/attachment authority.
type Status struct {
	Materialization domain.Materialization
	Storage         *ports.WorkingStorage
}

type StatusReader struct {
	Repository ports.MaterializationRepository
	Storage    ports.WorkingStorageProvider
}

// Status reads an already-authorized tenant-scoped Materialization without
// provisioning, restoring, releasing, or changing metadata. Callers must not hold
// a database transaction across provider I/O. Missing storage, including after
// release, returns ErrWorkingStorageNotFound alongside the persisted lifecycle.
// A concurrent metadata change returns StateConflict; callers may retry. This is
// an observation, not an atomic snapshot of PostgreSQL and the storage provider.
func (r StatusReader) Status(ctx context.Context, tenant, id uuid.UUID) (Status, error) {
	if err := ctx.Err(); err != nil {
		return Status{}, err
	}
	if r.Repository == nil || r.Storage == nil {
		return Status{}, errors.New("materialization status is not configured")
	}
	m, err := r.Repository.Get(ctx, tenant, id)
	if err != nil {
		return Status{}, err
	}
	if m.TenantID != tenant || m.ID != id {
		return Status{}, ports.ErrMaterializationNotFound
	}
	if err := m.Validate(); err != nil {
		return Status{}, err
	}
	result := Status{Materialization: m}
	if m.Handle == "" {
		return result, nil
	}
	storage, observationErr := r.Storage.Status(ctx, m)
	if err := ctx.Err(); err != nil {
		return result, err
	}
	current, err := r.Repository.Get(ctx, tenant, id)
	if err != nil {
		return Status{}, err
	}
	if current != m {
		return Status{}, ports.ErrMaterializationStateConflict
	}
	if observationErr != nil {
		return result, observationErr
	}
	if storage.Handle != m.Handle {
		return result, ports.ErrWorkingStorageConflict
	}
	switch storage.Phase {
	case ports.WorkingStoragePending, ports.WorkingStorageBound, ports.WorkingStorageLost, ports.WorkingStorageReleasing:
		result.Storage = &storage
		return result, nil
	default:
		return result, ports.ErrWorkingStorageUnavailable
	}
}
