package ports

import (
	"context"
	"errors"
	"time"

	"github.com/bdobrica/ThinkPixelWS/internal/domain"
	"github.com/google/uuid"
)

var ErrMaterializationLeaseNotRenewable = errors.New("materialization lease is not renewable")

var ErrWritableLeaseConflict = errors.New("workspace already has a current writable lease")
var ErrMaterializationLeaseIneligible = errors.New("materialization is not eligible for initial writable lease acquisition")

// MaterializationLeaseRepository reserves a Workspace writer slot and allocates
// its fence atomically. The caller must authorize the operation first; a holder
// reference is not authority. Acquisition returns only committed lease metadata.
// Acquisition retires expired slots atomically. Renewal requires the matching
// tenant, Materialization, lease, holder and current fence. Callers must reauthorize
// every renewal; these identifiers do not constitute a grant. authorityExpiresAt
// must come from fresh verified authority, never request input. Renew rejects a
// missing/expired deadline after acquiring locks and caps lease expiry at it.
// An explicit local authorizer must likewise supply a finite authority deadline.
type MaterializationLeaseRepository interface {
	Acquire(ctx context.Context, tenantID, materializationID, leaseID uuid.UUID, holder string) (domain.MaterializationLease, error)
	Renew(ctx context.Context, tenantID, materializationID, leaseID uuid.UUID, fence uint64, holder string, authorityExpiresAt time.Time) (domain.MaterializationLease, error)
	// Expire retires an expired writer for one Workspace and fences its nonterminal
	// Materialization. It is idempotent and does not allocate a new fence.
	Expire(ctx context.Context, tenantID, workspaceID uuid.UUID) (bool, error)
}
