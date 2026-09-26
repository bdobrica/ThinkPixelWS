package postgres

import (
	"context"
	"database/sql"
	"errors"
	"math"
	"time"

	"github.com/bdobrica/ThinkPixelWS/internal/domain"
	"github.com/bdobrica/ThinkPixelWS/internal/ports"
	"github.com/google/uuid"
)

type MaterializationWriterGuard struct{ tx *sql.Tx }

// NewMaterializationWriterGuard deliberately accepts only a transaction: a DB
// would release the fencing locks before the protected mutation could run.
func NewMaterializationWriterGuard(tx *sql.Tx) *MaterializationWriterGuard {
	return &MaterializationWriterGuard{tx: tx}
}

func (g *MaterializationWriterGuard) ValidateCheckpoint(ctx context.Context, writer ports.MaterializationWriter) error {
	_, err := g.validate(ctx, writer)
	return err
}

func (g *MaterializationWriterGuard) ValidateCommit(ctx context.Context, writer ports.MaterializationWriter, expectedHead uint64) error {
	if g.tx == nil {
		return errors.New("writer guard requires a transaction")
	}
	if expectedHead == 0 || expectedHead > math.MaxInt64 {
		return ports.ErrWorkspaceHeadConflict
	}
	var isolation string
	if err := g.tx.QueryRowContext(ctx, `SHOW transaction_isolation`).Scan(&isolation); err != nil {
		return err
	}
	if isolation != "serializable" {
		return errors.New("commit requires a serializable transaction")
	}
	head, err := g.validate(ctx, writer)
	if err != nil {
		return err
	}
	if !head.Valid || uint64(head.Int64) != expectedHead {
		return ports.ErrWorkspaceHeadConflict
	}
	return nil
}

func (g *MaterializationWriterGuard) validate(ctx context.Context, w ports.MaterializationWriter) (sql.NullInt64, error) {
	var head sql.NullInt64
	if g.tx == nil {
		return head, errors.New("writer guard requires a transaction")
	}
	if w.TenantID == uuid.Nil || w.WorkspaceID == uuid.Nil || w.MaterializationID == uuid.Nil || w.LeaseID == uuid.Nil || w.Fence == 0 || w.Fence > math.MaxInt64 {
		return head, ports.ErrMaterializationWriterConflict
	}
	// Same lock order as acquisition, renewal and expiry. Reading wall time only
	// after all locks also rejects a lease that expired while waiting for a lock.
	var fence uint64
	err := g.tx.QueryRowContext(ctx, `SELECT writer_fence,head_generation FROM thinkpixelws.workspaces
 WHERE tenant_id=$1 AND workspace_id=$2 FOR UPDATE`, w.TenantID, w.WorkspaceID).Scan(&fence, &head)
	if errors.Is(err, sql.ErrNoRows) {
		return head, ports.ErrMaterializationWriterConflict
	}
	if err != nil {
		return head, err
	}
	var leaseFence uint64
	var renewed, expires time.Time
	var released sql.NullTime
	var state domain.MaterializationState
	var mode domain.MaterializationMode
	err = g.tx.QueryRowContext(ctx, `SELECT l.fencing_token,l.renewed_at,l.expires_at,l.released_at,m.lifecycle_state,m.mode
 FROM thinkpixelws.materialization_leases l
 JOIN thinkpixelws.materializations m USING (tenant_id,workspace_id,materialization_id)
 WHERE l.tenant_id=$1 AND l.workspace_id=$2 AND l.materialization_id=$3 AND l.lease_id=$4
 FOR UPDATE OF l,m`, w.TenantID, w.WorkspaceID, w.MaterializationID, w.LeaseID).Scan(&leaseFence, &renewed, &expires, &released, &state, &mode)
	if errors.Is(err, sql.ErrNoRows) {
		return head, ports.ErrMaterializationWriterConflict
	}
	if err != nil {
		return head, err
	}
	var now time.Time
	if err := g.tx.QueryRowContext(ctx, `SELECT clock_timestamp()`).Scan(&now); err != nil {
		return head, err
	}
	if fence != w.Fence || leaseFence != w.Fence || released.Valid || !now.Before(expires) || now.Before(renewed) ||
		mode != domain.MaterializationReadWrite || (state != domain.MaterializationActive && state != domain.MaterializationCheckpointing) {
		return head, ports.ErrMaterializationWriterConflict
	}
	return head, nil
}

var _ ports.MaterializationWriterGuard = (*MaterializationWriterGuard)(nil)
