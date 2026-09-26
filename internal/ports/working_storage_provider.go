package ports

import (
	"context"
	"errors"

	"github.com/bdobrica/ThinkPixelWS/internal/domain"
)

var (
	ErrWorkingStorageNotFound    = errors.New("working storage not found")
	ErrWorkingStorageConflict    = errors.New("working storage ownership or specification conflict")
	ErrWorkingStorageUnavailable = errors.New("working storage unavailable")
)

type WorkingStoragePhase string

const (
	WorkingStoragePending   WorkingStoragePhase = "pending"
	WorkingStorageBound     WorkingStoragePhase = "bound"
	WorkingStorageLost      WorkingStoragePhase = "lost"
	WorkingStorageReleasing WorkingStoragePhase = "releasing"
)

type WorkingStorage struct {
	Handle domain.MaterializationHandle
	Phase  WorkingStoragePhase
}

// WorkingStorageProvider manages disposable hot storage only. Callers supply
// trusted, tenant-scoped repository records after authorization. Allocate requires
// PREPARING and an empty handle; retrying the same identity is idempotent. Persist
// the returned handle before using Status or Release. A bound volume is not a
// prepared Materialization: content restore, layout, lease/fence enforcement and
// read-only attachment are separate responsibilities. No method grants authority
// or mutates canonical Workspace metadata/content. Release is asynchronous and
// must only be called after execution has been detached and release authorized.
// Do not hold a database transaction open across provider I/O.
type WorkingStorageProvider interface {
	Allocate(context.Context, domain.Materialization) (WorkingStorage, error)
	Status(context.Context, domain.Materialization) (WorkingStorage, error)
	Release(context.Context, domain.Materialization) error
}
