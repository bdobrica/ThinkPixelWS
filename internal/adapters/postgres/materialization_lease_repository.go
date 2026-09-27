package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/bdobrica/ThinkPixelWS/internal/domain"
	"github.com/bdobrica/ThinkPixelWS/internal/ports"
	"github.com/google/uuid"
	"github.com/lib/pq"
)

// MaterializationLeaseRepository owns each operation's transaction so failures can
// never commit a fence increment without its lease. This is a persistence boundary;
// callers remain responsible for Workspace eligibility and governed authorization.
type MaterializationLeaseRepository struct{ db *sql.DB }

func NewMaterializationLeaseRepository(db *sql.DB) *MaterializationLeaseRepository {
	return &MaterializationLeaseRepository{db: db}
}

// Acquire reserves the initial writer slot for a REQUESTED, PREPARING or READY
// writable Materialization, retiring an expired slot atomically. It does not activate execution.
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
	if _, err := expireWriter(ctx, tx, tenantID, m.WorkspaceID); err != nil {
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

// lockLeaseWorkspace serializes renewal, expiry and acquisition without advancing
// the fence. All operations lock the Workspace before its lease/Materialization.
func lockLeaseWorkspace(ctx context.Context, tx *sql.Tx, tenantID, workspaceID uuid.UUID) (uint64, error) {
	var fence uint64
	err := tx.QueryRowContext(ctx, `SELECT writer_fence FROM thinkpixelws.workspaces
 WHERE tenant_id=$1 AND workspace_id=$2 FOR UPDATE`, tenantID, workspaceID).Scan(&fence)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, ports.ErrWorkspaceNotFound
	}
	return fence, err
}

// expireWriter requires the Workspace lock. Lock the current lease and its
// Materialization before reading wall time, including time spent waiting.
func expireWriter(ctx context.Context, tx *sql.Tx, tenantID, workspaceID uuid.UUID) (bool, error) {
	var leaseID, matID uuid.UUID
	var expires time.Time
	err := tx.QueryRowContext(ctx, `SELECT l.lease_id,l.materialization_id,l.expires_at
 FROM thinkpixelws.materialization_leases l
 JOIN thinkpixelws.materializations m USING (tenant_id,workspace_id,materialization_id)
 WHERE l.tenant_id=$1 AND l.workspace_id=$2 AND l.released_at IS NULL
 FOR UPDATE OF l,m`, tenantID, workspaceID).Scan(&leaseID, &matID, &expires)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	var now time.Time
	if err := tx.QueryRowContext(ctx, `SELECT clock_timestamp()`).Scan(&now); err != nil {
		return false, err
	}
	if now.Before(expires) {
		return false, nil
	}
	if _, err := tx.ExecContext(ctx, `UPDATE thinkpixelws.materialization_leases SET released_at=$3
 WHERE tenant_id=$1 AND lease_id=$2`, tenantID, leaseID, now); err != nil {
		return false, err
	}
	// Preserve terminal history; every live writer loses lifecycle eligibility.
	_, err = tx.ExecContext(ctx, `UPDATE thinkpixelws.materializations
 SET lifecycle_state='FENCED',clean_generation=NULL,state_version=state_version+1,updated_at=GREATEST(updated_at,$3)
 WHERE tenant_id=$1 AND materialization_id=$2
 AND lifecycle_state NOT IN ('FENCED','FAILED','RELEASED')`, tenantID, matID, now)
	return err == nil, err
}

func (r *MaterializationLeaseRepository) Expire(ctx context.Context, tenantID, workspaceID uuid.UUID) (bool, error) {
	if tenantID == uuid.Nil || workspaceID == uuid.Nil {
		return false, errors.New("tenant and workspace IDs are required")
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	if _, err := lockLeaseWorkspace(ctx, tx, tenantID, workspaceID); err != nil {
		return false, err
	}
	expired, err := expireWriter(ctx, tx, tenantID, workspaceID)
	if err != nil {
		return false, err
	}
	if err := tx.Commit(); err != nil {
		return false, err
	}
	return expired, nil
}

func (r *MaterializationLeaseRepository) Renew(ctx context.Context, tenantID, materializationID, leaseID uuid.UUID, fence uint64, holder string, authorityExpiresAt time.Time) (domain.MaterializationLease, error) {
	empty := domain.MaterializationLease{}
	if authorityExpiresAt.IsZero() || tenantID == uuid.Nil || materializationID == uuid.Nil || leaseID == uuid.Nil || fence < 1 || fence > math.MaxInt64 || holder == "" {
		return empty, ports.ErrMaterializationLeaseNotRenewable
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return empty, err
	}
	defer tx.Rollback()
	var workspaceID uuid.UUID
	err = tx.QueryRowContext(ctx, `SELECT workspace_id FROM thinkpixelws.materialization_leases
 WHERE tenant_id=$1 AND materialization_id=$2 AND lease_id=$3 AND fencing_token=$4 AND holder=$5`,
		tenantID, materializationID, leaseID, fence, holder).Scan(&workspaceID)
	if errors.Is(err, sql.ErrNoRows) {
		return empty, ports.ErrMaterializationLeaseNotRenewable
	}
	if err != nil {
		return empty, err
	}
	currentFence, err := lockLeaseWorkspace(ctx, tx, tenantID, workspaceID)
	if err != nil {
		return empty, err
	}
	var lease domain.MaterializationLease
	var state domain.MaterializationState
	err = tx.QueryRowContext(ctx, `SELECT l.tenant_id,l.lease_id,l.workspace_id,l.materialization_id,
 l.fencing_token,l.holder,l.issued_at,l.renewed_at,l.expires_at,l.released_at,m.lifecycle_state
 FROM thinkpixelws.materialization_leases l
 JOIN thinkpixelws.materializations m USING (tenant_id,workspace_id,materialization_id)
 WHERE l.tenant_id=$1 AND l.lease_id=$2 FOR UPDATE OF l,m`, tenantID, leaseID).Scan(
		&lease.TenantID, &lease.ID, &lease.WorkspaceID, &lease.MaterializationID,
		&lease.FencingToken, &lease.Holder, &lease.IssuedAt, &lease.RenewedAt, &lease.ExpiresAt, &lease.ReleasedAt, &state)
	if err != nil {
		return empty, err
	}
	var now time.Time
	if err := tx.QueryRowContext(ctx, `SELECT clock_timestamp()`).Scan(&now); err != nil {
		return empty, err
	}
	if !now.Before(authorityExpiresAt) || lease.MaterializationID != materializationID || lease.Holder != holder || lease.FencingToken != fence ||
		currentFence != fence || lease.ReleasedAt != nil || !now.Before(lease.ExpiresAt) || now.Before(lease.RenewedAt) ||
		(state != domain.MaterializationRequested && state != domain.MaterializationPreparing &&
			state != domain.MaterializationReady && state != domain.MaterializationActive && state != domain.MaterializationCheckpointing) {
		return empty, ports.ErrMaterializationLeaseNotRenewable
	}
	lease.IssuedAt = lease.IssuedAt.UTC()
	lease.RenewedAt = now.UTC()
	lease.ExpiresAt = now.UTC().Add(domain.DefaultMaterializationLeaseDuration)
	if authorityExpiresAt.Before(lease.ExpiresAt) {
		lease.ExpiresAt = authorityExpiresAt.UTC()
	}
	if _, err := tx.ExecContext(ctx, `UPDATE thinkpixelws.materialization_leases
 SET renewed_at=$3,expires_at=$4 WHERE tenant_id=$1 AND lease_id=$2`,
		tenantID, leaseID, lease.RenewedAt, lease.ExpiresAt); err != nil {
		return empty, err
	}
	if err := tx.Commit(); err != nil {
		return empty, err
	}
	return lease, nil
}
