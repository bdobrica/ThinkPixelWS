package materialization

import (
	"context"
	"errors"

	"github.com/bdobrica/ThinkPixelWS/internal/domain"
	"github.com/bdobrica/ThinkPixelWS/internal/ports"
	"github.com/bdobrica/ThinkPixelWS/internal/ports/clock"
	"github.com/google/uuid"
)

// Recoverer resumes persisted intent without relying on process-local progress.
// Callers select tenant-scoped IDs and re-establish authorization, retention and
// execution-detachment preconditions before invoking it. A restart is not new
// attachment authority and must not renew leases or reset fencing tokens.
type Recoverer struct {
	Repository ports.MaterializationRepository
	Storage    ports.WorkingStorageProvider
	Content    ports.MaterializationContentPreparer
	Clock      clock.Clock
}

// Recover performs one retryable step. REQUESTED/PREPARING resumes preparation;
// RELEASING resumes deletion. READY/ACTIVE and terminal records are preserved
// without provider I/O, including working edits and storage identity. CHECKPOINTING
// returns a state conflict until checkpoint recovery exists; it is never reset.
// Pending work returns its persisted state with no error, not a completion claim.
// Repository writes must commit independently with required audit/outbox, never
// in a transaction spanning provider I/O. Scanning/scheduling belongs to callers.
func (r Recoverer) Recover(ctx context.Context, tenant, id uuid.UUID) (domain.Materialization, error) {
	if err := ctx.Err(); err != nil {
		return domain.Materialization{}, err
	}
	if r.Repository == nil {
		return domain.Materialization{}, errors.New("materialization recovery is not configured")
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
	switch m.State {
	case domain.MaterializationRequested, domain.MaterializationPreparing:
		return (Preparer{Repository: r.Repository, Storage: r.Storage, Content: r.Content, Clock: r.Clock}).Prepare(ctx, tenant, id)
	case domain.MaterializationReleasing:
		return (Releaser{Repository: r.Repository, Storage: r.Storage, Clock: r.Clock}).Release(ctx, tenant, id)
	case domain.MaterializationReady, domain.MaterializationActive,
		domain.MaterializationReleased, domain.MaterializationFailed, domain.MaterializationFenced:
		return m, nil
	default:
		return m, ports.ErrMaterializationStateConflict
	}
}
