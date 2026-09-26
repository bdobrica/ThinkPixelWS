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

type Preparer struct {
	Repository ports.MaterializationRepository
	Storage    ports.WorkingStorageProvider
	Content    ports.MaterializationContentPreparer
	Clock      clock.Clock
}

// Prepare advances one already-authorized, tenant-scoped request. Callers supply
// short, independently committed repository mutations (with required audit/outbox),
// never a transaction held across this call. Execution remains detached throughout;
// READY grants neither a writer lease nor AR attachment authority.
//
// Pending storage/content returns PREPARING without an error. Retry the same ID.
// Errors preserve the binding and progress, never delete storage or revive a
// terminal record. Concurrent lifecycle changes are rejected by compare-and-swap.
func (p Preparer) Prepare(ctx context.Context, tenant, id uuid.UUID) (domain.Materialization, error) {
	if err := ctx.Err(); err != nil {
		return domain.Materialization{}, err
	}
	if p.Repository == nil || p.Storage == nil || p.Content == nil || p.Clock == nil {
		return domain.Materialization{}, errors.New("materialization preparation is not configured")
	}
	m, err := p.Repository.Get(ctx, tenant, id)
	if err != nil {
		return domain.Materialization{}, err
	}
	if err := m.Validate(); err != nil {
		return domain.Materialization{}, err
	}
	if m.TenantID != tenant || m.ID != id {
		return domain.Materialization{}, ports.ErrMaterializationNotFound
	}
	if m.State == domain.MaterializationReady && m.Handle != "" {
		return m, nil
	}
	if m.State != domain.MaterializationRequested && m.State != domain.MaterializationPreparing {
		return m, ports.ErrMaterializationStateConflict
	}
	transition := func(ctx context.Context, next domain.MaterializationState) error {
		now := p.Clock.Now().UTC().Truncate(time.Microsecond)
		updated, err := m.TransitionState(next, m.StateVersion, now)
		if err != nil {
			return err
		}
		if err := p.Repository.TransitionState(ctx, tenant, id, m.State, next, m.StateVersion, now); err != nil {
			return err
		}
		m = updated
		return nil
	}
	if m.State == domain.MaterializationRequested {
		if err := transition(ctx, domain.MaterializationPreparing); err != nil {
			return m, err
		}
	}
	if m.Handle == "" {
		storage, err := p.Storage.Allocate(ctx, m)
		if err != nil {
			return m, err
		}
		now := p.Clock.Now().UTC().Truncate(time.Microsecond)
		updated, err := m.Bind(storage.Handle, m.StateVersion, now)
		if err != nil {
			return m, err
		}
		if err := p.Repository.Bind(ctx, tenant, id, storage.Handle, m.StateVersion, now); err != nil {
			return m, err
		}
		m = updated
	}
	checkStorage := func(ctx context.Context) (ports.WorkingStoragePhase, error) {
		storage, err := p.Storage.Status(ctx, m)
		if err != nil {
			return "", err
		}
		if storage.Handle != m.Handle {
			return "", ports.ErrWorkingStorageConflict
		}
		if storage.Phase != ports.WorkingStoragePending && storage.Phase != ports.WorkingStorageBound {
			return "", ports.ErrWorkingStorageConflict
		}
		return storage.Phase, nil
	}
	if _, err := checkStorage(ctx); err != nil {
		return m, err
	}
	completed := false
	prepared, err := p.Content.Prepare(ctx, m, func(ctx context.Context) error {
		current, err := p.Repository.Get(ctx, tenant, id)
		if err != nil {
			return err
		}
		if current != m {
			return ports.ErrMaterializationStateConflict
		}
		_, err = checkStorage(ctx)
		return err
	}, func(callbackCtx context.Context) error {
		if err := callbackCtx.Err(); err != nil {
			return err
		}
		phase, err := checkStorage(callbackCtx)
		if err != nil {
			return err
		}
		if phase != ports.WorkingStorageBound {
			return ports.ErrWorkingStorageUnavailable
		}
		if err := transition(callbackCtx, domain.MaterializationReady); err != nil {
			return err
		}
		completed = true
		return nil
	})
	if err != nil {
		return m, err
	}
	// Only our successful persisted transition is evidence of readiness.
	if prepared != completed {
		return m, errors.New("inconsistent content preparation result")
	}
	return m, nil
}
