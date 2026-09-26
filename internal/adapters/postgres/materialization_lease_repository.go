package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/bdobrica/ThinkPixelWS/internal/domain"
	"github.com/bdobrica/ThinkPixelWS/internal/ports"
	"github.com/google/uuid"
	"github.com/lib/pq"
)

// MaterializationLeaseRepository owns acquisition's transaction so failures can
// never commit a fence increment without its lease. This is a persistence boundary;
// callers remain responsible for Workspace eligibility and governed authorization.
type MaterializationLeaseRepository struct{ db *sql.DB }

func NewMaterializationLeaseRepository(db *sql.DB) *MaterializationLeaseRepository {
	return &MaterializationLeaseRepository{db: db}
}

// Acquire reserves the initial writer slot for a REQUESTED, PREPARING or READY
// writable Materialization. It does not activate execution or reclaim expired slots.
func (r *MaterializationLeaseRepository) Acquire(ctx context.Context, tenantID, materializationID, leaseID uuid.UUID, holder string) (domain.MaterializationLease, error) {
	empty := domain.MaterializationLease{}
	if tenantID == uuid.Nil || materializationID == uuid.Nil {
		return empty, errors.New("tenant and materialization IDs are required")
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return empty, fmt.Errorf("begin lease acquisition: %w", err)
	}
	defer tx.Rollback()
	m, err := NewMaterializationRepository(tx).Get(ctx, tenantID, materializationID)
	if err != nil {
		return empty, err
	}
	// Lock the Workspace first; every acquisition for it follows the same order.
	fence, err := NewWorkspaceRepository(tx).AdvanceWriterFence(ctx, tenantID, m.WorkspaceID)
	if err != nil {
		return empty, err
	}
	var state domain.MaterializationState
	var mode domain.MaterializationMode
	err = tx.QueryRowContext(ctx, `SELECT lifecycle_state, mode FROM thinkpixelws.materializations
 WHERE tenant_id=$1 AND materialization_id=$2 AND workspace_id=$3 FOR UPDATE`, tenantID, materializationID, m.WorkspaceID).Scan(&state, &mode)
	if err != nil {
		return empty, fmt.Errorf("lock materialization: %w", err)
	}
	if mode != domain.MaterializationReadWrite || (state != domain.MaterializationRequested && state != domain.MaterializationPreparing && state != domain.MaterializationReady) {
		return empty, ports.ErrMaterializationLeaseIneligible
	}
	// Use database wall time after locks, so time spent waiting does not consume TTL.
	var now time.Time
	if err := tx.QueryRowContext(ctx, `SELECT clock_timestamp()`).Scan(&now); err != nil {
		return empty, err
	}
	lease, err := (domain.NewMaterializationLease{ID: leaseID, FencingToken: fence, Holder: holder}).MaterializationLease(m, now)
	if err != nil {
		return empty, err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO thinkpixelws.materialization_leases
 (tenant_id,lease_id,workspace_id,materialization_id,fencing_token,holder,issued_at,renewed_at,expires_at)
 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)`, tenantID, lease.ID, lease.WorkspaceID, lease.MaterializationID, lease.FencingToken, lease.Holder, lease.IssuedAt, lease.RenewedAt, lease.ExpiresAt)
	if err != nil {
		var pgError *pq.Error
		if errors.As(err, &pgError) && pgError.Code == "23505" && pgError.Constraint == "materialization_leases_current_writer" {
			return empty, ports.ErrWritableLeaseConflict
		}
		return empty, fmt.Errorf("insert writable lease: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return empty, fmt.Errorf("commit lease acquisition: %w", err)
	}
	return lease, nil
}

var _ ports.MaterializationLeaseRepository = (*MaterializationLeaseRepository)(nil)
