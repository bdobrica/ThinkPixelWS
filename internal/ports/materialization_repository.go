package ports

import (
	"context"
	"errors"
	"time"

	"github.com/bdobrica/ThinkPixelWS/internal/domain"
	"github.com/google/uuid"
)

var (
	ErrMaterializationNotFound = errors.New("materialization not found")
	// Conflict also covers absent/foreign-tenant records without revealing existence.
	ErrMaterializationStateConflict = errors.New("materialization state conflict")
)

// MaterializationRepository persists requested realizations, never authority.
// Create accepts only the initial REQUESTED state. Callers must authorize the
// operation and use a transaction to couple the mutation with audit/outbox.
// Lifecycle mutations do not acquire a writer lease or enforce its fence.
// Multiple read-only realizations of the same Workspace generation/target may
// coexist, including alongside a writable lease, without allocating a fence.
type MaterializationRepository interface {
	Create(context.Context, uuid.UUID, domain.Materialization) error
	Get(context.Context, uuid.UUID, uuid.UUID) (domain.Materialization, error)
	// Bind records an immutable provider handle while PREPARING, incrementing the
	// shared state version. Absent/foreign/stale/already-bound rows return StateConflict.
	Bind(context.Context, uuid.UUID, uuid.UUID, domain.MaterializationHandle, uint64, time.Time) error
	TransitionState(context.Context, uuid.UUID, uuid.UUID, domain.MaterializationState, domain.MaterializationState, uint64, time.Time) error
}
