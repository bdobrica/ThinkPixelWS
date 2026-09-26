package ports

import (
	"context"
	"errors"

	"github.com/bdobrica/ThinkPixelWS/internal/domain"
	"github.com/google/uuid"
)

var ErrMaterializationNotFound = errors.New("materialization not found")

// MaterializationRepository persists requested realizations, never authority.
// Create accepts only the initial REQUESTED state. Callers must authorize the
// operation and use a transaction to couple the mutation with audit/outbox.
// Lifecycle mutations and writer acquisition are separate operations.
type MaterializationRepository interface {
	Create(context.Context, uuid.UUID, domain.Materialization) error
	Get(context.Context, uuid.UUID, uuid.UUID) (domain.Materialization, error)
}
