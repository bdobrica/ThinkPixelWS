package postgres

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/bdobrica/ThinkPixelWS/internal/domain"
	"github.com/bdobrica/ThinkPixelWS/internal/ports"
	"github.com/google/uuid"
)

// WorkspaceCreator uses PostgreSQL's unique-key serialization to coordinate
// retries across processes. All mutation records commit in this transaction.
type WorkspaceCreator struct{ DB *sql.DB }

func (c WorkspaceCreator) CreateWorkspace(ctx context.Context, in ports.WorkspaceCreation) (domain.Workspace, error) {
	w := in.Workspace
	if err := w.Validate(); err != nil {
		return domain.Workspace{}, err
	}
	if err := in.Audit.Validate(); err != nil {
		return domain.Workspace{}, err
	}
	if err := in.Outbox.Validate(); err != nil {
		return domain.Workspace{}, err
	}
	if in.Audit.TenantID != w.TenantID || in.Outbox.TenantID != w.TenantID || in.Audit.WorkspaceID == nil || *in.Audit.WorkspaceID != w.ID || in.Outbox.AggregateID != w.ID || in.Audit.TransactionID != in.Outbox.TransactionID || in.Audit.ActorPrincipal != in.Principal {
		return domain.Workspace{}, errors.New("creation records have mismatched scope")
	}
	tx, err := c.DB.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return domain.Workspace{}, err
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(ctx, `INSERT INTO thinkpixelws.idempotency_records
 (tenant_id,principal,operation,key_hash,request_digest,status,created_at,updated_at,expires_at)
 VALUES ($1,$2,'createWorkspace',$3,$4,'IN_PROGRESS',$5,$5,$6)
 ON CONFLICT (tenant_id,principal,operation,key_hash) DO NOTHING`, w.TenantID, in.Principal, in.KeyHash.String(), in.RequestDigest.String(), w.CreatedAt, w.CreatedAt.Add(24*time.Hour))
	if err != nil {
		return domain.Workspace{}, err
	}
	inserted, err := result.RowsAffected()
	if err != nil {
		return domain.Workspace{}, err
	}
	if inserted == 0 {
		var digest, status, ref string
		err = tx.QueryRowContext(ctx, `SELECT request_digest,status,result_ref FROM thinkpixelws.idempotency_records WHERE tenant_id=$1 AND principal=$2 AND operation='createWorkspace' AND key_hash=$3`, w.TenantID, in.Principal, in.KeyHash.String()).Scan(&digest, &status, &ref)
		if err != nil {
			return domain.Workspace{}, err
		}
		if digest != in.RequestDigest.String() {
			return domain.Workspace{}, ports.ErrIdempotencyConflict
		}
		if status != "COMPLETED" {
			return domain.Workspace{}, errors.New("creation is incomplete")
		}
		id, err := uuid.Parse(ref)
		if err != nil {
			return domain.Workspace{}, err
		}
		// Initial metadata comes from the digest-matched input, not mutable stored
		// fields. Only immutable identity and creation time are read for replay.
		err = tx.QueryRowContext(ctx, `SELECT created_at FROM thinkpixelws.workspaces WHERE tenant_id=$1 AND workspace_id=$2`, w.TenantID, id).Scan(&w.CreatedAt)
		if err != nil {
			return domain.Workspace{}, err
		}
		w.ID = id
		w.UpdatedAt = w.CreatedAt
		return w, tx.Commit()
	}
	if err = NewWorkspaceRepository(tx).Create(ctx, w.TenantID, w); err != nil {
		return domain.Workspace{}, err
	}
	a := in.Audit
	_, err = tx.ExecContext(ctx, `INSERT INTO thinkpixelws.audit_events
 (tenant_id,workspace_id,audit_event_id,transaction_id,actor_principal,action,target_kind,target_id,decision,outcome,trace_id,request_id,metadata_schema,metadata,occurred_at)
 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,NULLIF($11,''),NULLIF($12,''),$13,$14::jsonb,$15)`, a.TenantID, a.WorkspaceID, a.ID, a.TransactionID, a.ActorPrincipal, a.Action, a.TargetKind, a.TargetID, a.Decision, a.Outcome, a.TraceID, a.RequestID, a.MetadataSchema, string(a.Metadata), a.OccurredAt)
	if err != nil {
		return domain.Workspace{}, err
	}
	o := in.Outbox
	_, err = tx.ExecContext(ctx, `INSERT INTO thinkpixelws.outbox_messages
 (tenant_id,event_id,transaction_id,aggregate_kind,aggregate_id,aggregate_version,sequence,event_type,event_version,payload_schema,payload,occurred_at,available_at)
 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11::jsonb,$12,$13)`, o.TenantID, o.EventID, o.TransactionID, o.AggregateKind, o.AggregateID, o.AggregateVersion, o.Sequence, o.EventType, o.EventVersion, o.PayloadSchema, string(o.Payload), o.OccurredAt, o.AvailableAt)
	if err != nil {
		return domain.Workspace{}, err
	}
	_, err = tx.ExecContext(ctx, `UPDATE thinkpixelws.idempotency_records SET status='COMPLETED',response_status=201,result_ref=$4 WHERE tenant_id=$1 AND principal=$2 AND operation='createWorkspace' AND key_hash=$3`, w.TenantID, in.Principal, in.KeyHash.String(), w.ID.String())
	if err != nil {
		return domain.Workspace{}, err
	}
	return w, tx.Commit()
}

var _ ports.WorkspaceCreator = WorkspaceCreator{}
