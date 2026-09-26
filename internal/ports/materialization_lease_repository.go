package ports

import (
	"context"
	"errors"

	"github.com/bdobrica/ThinkPixelWS/internal/domain"
	"github.com/google/uuid"
)

var ErrWritableLeaseConflict = errors.New("workspace already has a current writable lease")
var ErrMaterializationLeaseIneligible = errors.New("materialization is not eligible for initial writable lease acquisition")

// MaterializationLeaseRepository reserves a Workspace writer slot and allocates
// its fence atomically. The caller must authorize the operation first; a holder
// reference is not authority. Acquisition returns only committed lease metadata.
// Expired but unreleased leases still conflict until expiry handling retires them.
type MaterializationLeaseRepository interface {
	Acquire(ctx context.Context, tenantID, materializationID, leaseID uuid.UUID, holder string) (domain.MaterializationLease, error)
}
