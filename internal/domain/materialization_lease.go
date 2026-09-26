package domain

import (
	"errors"
	"math"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
)

const (
	DefaultMaterializationLeaseDuration        = 60 * time.Second
	DefaultMaterializationLeaseRenewalInterval = 20 * time.Second
)

// MaterializationLease records a writable lease. Constructing or validating this
// value does not acquire a lease or grant execution authority. Acquisition must
// separately serialize writers and allocate the Workspace fencing token.
// Holder is a stable, non-secret holder reference, never a credential or grant.
type MaterializationLease struct {
	TenantID          uuid.UUID
	ID                uuid.UUID
	WorkspaceID       uuid.UUID
	MaterializationID uuid.UUID
	FencingToken      uint64
	Holder            string
	IssuedAt          time.Time
	RenewedAt         time.Time
	ExpiresAt         time.Time
	ReleasedAt        *time.Time
}

type NewMaterializationLease struct {
	ID           uuid.UUID
	FencingToken uint64
	Holder       string
}

// MaterializationLease builds initial metadata with the ADR-0002 default duration.
// The supplied Materialization must be writable; its lifecycle and authority are
// checked by the acquisition operation, not inferred from this metadata.
func (input NewMaterializationLease) MaterializationLease(m Materialization, now time.Time) (MaterializationLease, error) {
	if err := m.Validate(); err != nil {
		return MaterializationLease{}, err
	}
	if m.Mode != MaterializationReadWrite {
		return MaterializationLease{}, errors.New("lease requires a writable materialization")
	}
	lease := MaterializationLease{
		TenantID: m.TenantID, ID: input.ID, WorkspaceID: m.WorkspaceID, MaterializationID: m.ID,
		FencingToken: input.FencingToken, Holder: input.Holder,
		IssuedAt: now.UTC(), RenewedAt: now.UTC(), ExpiresAt: now.UTC().Add(DefaultMaterializationLeaseDuration),
	}
	if err := lease.Validate(); err != nil {
		return MaterializationLease{}, err
	}
	return lease, nil
}

func (lease MaterializationLease) Validate() error {
	for _, id := range []uuid.UUID{lease.TenantID, lease.ID, lease.WorkspaceID, lease.MaterializationID} {
		if id.Version() != 7 || id.Variant() != uuid.RFC4122 {
			return errors.New("lease IDs must be UUIDv7")
		}
	}
	if lease.FencingToken < 1 || lease.FencingToken > math.MaxInt64 {
		return errors.New("lease fencing token is invalid")
	}
	if !utf8.ValidString(lease.Holder) || !validBoundedSourceValue(lease.Holder, 256) {
		return errors.New("lease holder is invalid")
	}
	if lease.IssuedAt.IsZero() || lease.RenewedAt.IsZero() || lease.ExpiresAt.IsZero() ||
		lease.RenewedAt.Before(lease.IssuedAt) || !lease.ExpiresAt.After(lease.RenewedAt) {
		return errors.New("lease timestamps are invalid")
	}
	// Cleanup may record release after expiry, but never before the last renewal.
	if lease.ReleasedAt != nil && (lease.ReleasedAt.IsZero() || lease.ReleasedAt.Before(lease.RenewedAt)) {
		return errors.New("lease release time is invalid")
	}
	return nil
}
