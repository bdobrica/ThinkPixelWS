package materialization

import (
	"context"
	"errors"
	"time"

	"github.com/bdobrica/ThinkPixelWS/internal/domain"
	"github.com/bdobrica/ThinkPixelWS/internal/ports"
	"github.com/bdobrica/ThinkPixelWS/internal/ports/clock"
	"github.com/google/uuid"
)

type Releaser struct {
	Repository ports.MaterializationRepository
	Storage    ports.WorkingStorageProvider
	Clock      clock.Clock
}

// Release advances an authorized, execution-detached Materialization through
// RELEASING to RELEASED. The caller must prevent attachment throughout release,
// preserve any required working changes beforehand, and enforce retention and
// lease policy. This operation does not checkpoint, retire leases, or delete
// canonical Workspace metadata, generations, snapshots, or keys.
//
// Repository mutations must be independently committed with required audit/outbox;
// never hold a database transaction across provider I/O. Pending deletion returns
// RELEASING without an error. Errors retain progress for retry with the same ID.
// FAILED/FENCED cleanup belongs to reconciliation, not lifecycle revival here.
func (r Releaser) Release(ctx context.Context, tenant, id uuid.UUID) (domain.Materialization, error) {
	if err := ctx.Err(); err != nil {
		return domain.Materialization{}, err
	}
	if r.Repository == nil || r.Storage == nil || r.Clock == nil {
		return domain.Materialization{}, errors.New("materialization release is not configured")
	}
	m, err := r.Repository.Get(ctx, tenant, id)
	if err != nil {
		return domain.Materialization{}, err
	}
	if m.TenantID != tenant || m.ID != id {
		return domain.Materialization{}, ports.ErrMaterializationNotFound
	}
	if err := m.Validate(); err != nil {
		return domain.Materialization{}, err
	}
	if m.State == domain.MaterializationReleased {
		return m, nil
	}
	if m.State != domain.MaterializationReady && m.State != domain.MaterializationActive && m.State != domain.MaterializationReleasing {
		return m, ports.ErrMaterializationStateConflict
	}
	// Never infer absence from a missing binding: allocation may have succeeded
	// before its handle was persisted. Orphan cleanup is a separate operation.
	if m.Handle == "" {
		return m, ports.ErrWorkingStorageConflict
	}
	transition := func(next domain.MaterializationState) error {
		now := r.Clock.Now().UTC().Truncate(time.Microsecond)
		updated, err := m.TransitionState(next, m.StateVersion, now)
		if err != nil {
			return err
		}
		if err := r.Repository.TransitionState(ctx, tenant, id, m.State, next, m.StateVersion, now); err != nil {
			return err
		}
		m = updated
		return nil
	}
	if m.State != domain.MaterializationReleasing {
		if err := transition(domain.MaterializationReleasing); err != nil {
			return m, err
		}
	}
	if err := r.Storage.Release(ctx, m); err != nil {
		return m, err
	}
	storage, observationErr := r.Storage.Status(ctx, m)
	if err := ctx.Err(); err != nil {
		return m, err
	}
	current, err := r.Repository.Get(ctx, tenant, id)
	if err != nil {
		return m, err
	}
	if current != m {
		return m, ports.ErrMaterializationStateConflict
	}
	if errors.Is(observationErr, ports.ErrWorkingStorageNotFound) {
		if err := transition(domain.MaterializationReleased); err != nil {
			return m, err
		}
		return m, nil
	}
	if observationErr != nil {
		return m, observationErr
	}
	if storage.Handle != m.Handle {
		return m, ports.ErrWorkingStorageConflict
	}
	// A successful delete request is not evidence that asynchronous cleanup has
	// finished. Only an authoritative not-found observation completes release.
	return m, nil
}
