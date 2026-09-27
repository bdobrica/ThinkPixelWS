package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"math"
	"time"

	"github.com/bdobrica/ThinkPixelWS/internal/domain"
	"github.com/bdobrica/ThinkPixelWS/internal/ports"
	"github.com/bdobrica/ThinkPixelWS/internal/security"
	"github.com/google/uuid"
)

// GenerationCommitter is the metadata publication boundary. Provider IO must
// finish before entry; the writer guard is checked again here under DB locks.
type GenerationCommitter struct{ DB *sql.DB }

func (c GenerationCommitter) Commit(ctx context.Context, in ports.GenerationCommit) (domain.WorkspaceGeneration, error) {
	var zero domain.WorkspaceGeneration
	if err := ctx.Err(); err != nil {
		return zero, err
	}
	if in.AuthorityExpiresAt.IsZero() {
		return zero, security.ErrExecutionAuthority
	}
	if in.ExpectedHead == 0 || in.ExpectedHead >= math.MaxInt64 {
		return zero, ports.ErrWorkspaceHeadConflict
	}
	if in.MaterializationVersion == 0 || in.MaterializationVersion > math.MaxInt64 {
		return zero, ports.ErrMaterializationWriterConflict
	}
	g, err := (domain.NewWorkspaceGeneration{
		TenantID: in.Writer.TenantID, WorkspaceID: in.Writer.WorkspaceID,
		ID: in.GenerationID, Number: in.ExpectedHead + 1, ParentNumber: &in.ExpectedHead,
		ManifestDigest: in.ManifestDigest, Durability: in.Durability,
		ComponentReferences: in.ComponentReferences,
		CreatedByPrincipal:  in.Principal, CreatedByRun: in.RunID, CreatedByExecution: in.ExecutionID,
	}).WorkspaceGeneration(time.Now().UTC())
	if err != nil {
		return zero, err
	}
	// Persist an explicit empty set for an empty Workspace.
	if g.ComponentReferences == nil {
		g.ComponentReferences = []domain.GenerationComponentReference{}
	}
	references, err := json.Marshal(g.ComponentReferences)
	if err != nil {
		return zero, err
	}
	if c.DB == nil {
		return zero, errors.New("generation persistence is not configured")
	}
	tx, err := c.DB.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return zero, err
	}
	defer tx.Rollback()
	guard := NewMaterializationWriterGuard(tx)
	if err := guard.ValidateCommit(ctx, in.Writer, in.ExpectedHead); err != nil {
		return zero, err
	}
	if err := validateCommitAuthority(ctx, tx, in.AuthorityExpiresAt); err != nil {
		return zero, err
	}
	var version uint64
	var cleanGeneration uint64
	var state domain.MaterializationState
	if err := tx.QueryRowContext(ctx, `SELECT state_version,lifecycle_state,COALESCE(clean_generation,0) FROM thinkpixelws.materializations
 WHERE tenant_id=$1 AND workspace_id=$2 AND materialization_id=$3`, g.TenantID, g.WorkspaceID, in.Writer.MaterializationID).Scan(&version, &state, &cleanGeneration); err != nil {
		return zero, err
	}
	if version != in.MaterializationVersion {
		return zero, ports.ErrMaterializationWriterConflict
	}
	if (in.MarkClean && state != domain.MaterializationCheckpointing) ||
		((in.MarkClean || cleanGeneration != 0) && version == math.MaxInt64) {
		return zero, ports.ErrMaterializationWriterConflict
	}
	var components, matched int
	if err := tx.QueryRowContext(ctx, `SELECT count(*),count(*) FILTER (WHERE component_id IN
 (SELECT (value->>'componentId')::uuid FROM jsonb_array_elements($3::jsonb)))
 FROM thinkpixelws.workspace_components WHERE tenant_id=$1 AND workspace_id=$2`, g.TenantID, g.WorkspaceID, string(references)).Scan(&components, &matched); err != nil {
		return zero, err
	}
	if components != len(g.ComponentReferences) || matched != components {
		return zero, errors.New("generation references must cover exactly the Workspace components")
	}
	if err := tx.QueryRowContext(ctx, `SELECT clock_timestamp()`).Scan(&g.CreatedAt); err != nil {
		return zero, err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO thinkpixelws.workspace_generations
 (tenant_id,workspace_id,generation,generation_id,parent_generation,state,manifest_digest,durability,created_by_principal,created_by_execution_id,created_by_run_id,created_at,component_references)
 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13::jsonb)`, g.TenantID, g.WorkspaceID, g.Number, g.ID, g.ParentNumber, g.State, g.ManifestDigest.String(), g.Durability, g.CreatedByPrincipal, g.CreatedByExecution, g.CreatedByRun, g.CreatedAt, string(references))
	if err != nil {
		return zero, err
	}
	_, err = tx.ExecContext(ctx, `UPDATE thinkpixelws.workspaces SET head_generation=$3,updated_at=GREATEST(updated_at,$4)
 WHERE tenant_id=$1 AND workspace_id=$2`, g.TenantID, g.WorkspaceID, g.Number, g.CreatedAt)
	if err != nil {
		return zero, err
	}
	if in.MarkClean || cleanGeneration != 0 {
		var clean any
		if in.MarkClean {
			clean = g.Number
		}
		_, err = tx.ExecContext(ctx, `UPDATE thinkpixelws.materializations
 SET clean_generation=$4,state_version=state_version+1,updated_at=GREATEST(updated_at,$5)
 WHERE tenant_id=$1 AND workspace_id=$2 AND materialization_id=$3`, g.TenantID, g.WorkspaceID, in.Writer.MaterializationID, clean, g.CreatedAt)
		if err != nil {
			return zero, err
		}
	}
	transactionID, err := uuid.NewV7()
	if err != nil {
		return zero, err
	}
	auditID, err := uuid.NewV7()
	if err != nil {
		return zero, err
	}
	eventID, err := uuid.NewV7()
	if err != nil {
		return zero, err
	}
	payload, err := json.Marshal(struct {
		WorkspaceID       uuid.UUID `json:"workspaceId"`
		MaterializationID uuid.UUID `json:"materializationId"`
		GenerationID      uuid.UUID `json:"generationId"`
		Generation        uint64    `json:"generation"`
	}{g.WorkspaceID, in.Writer.MaterializationID, g.ID, g.Number})
	if err != nil {
		return zero, err
	}
	a, err := (domain.NewAuditEvent{
		TenantID: g.TenantID, WorkspaceID: &g.WorkspaceID, ID: auditID, TransactionID: transactionID,
		ActorPrincipal: in.Principal, Action: "workspace.commit", TargetKind: "generation", TargetID: g.ID.String(),
		Decision: "allow", Outcome: "success", RequestID: in.RequestID, TraceID: in.TraceID,
		MetadataSchema: "generation-committed.v1", Metadata: payload,
	}).AuditEvent(g.CreatedAt)
	if err != nil {
		return zero, err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO thinkpixelws.audit_events
 (tenant_id,workspace_id,audit_event_id,transaction_id,actor_principal,action,target_kind,target_id,decision,outcome,trace_id,request_id,metadata_schema,metadata,occurred_at)
 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,NULLIF($11,''),NULLIF($12,''),$13,$14::jsonb,$15)`, a.TenantID, a.WorkspaceID, a.ID, a.TransactionID, a.ActorPrincipal, a.Action, a.TargetKind, a.TargetID, a.Decision, a.Outcome, a.TraceID, a.RequestID, a.MetadataSchema, string(a.Metadata), a.OccurredAt)
	if err != nil {
		return zero, err
	}
	// Each immutable generation is its own aggregate, avoiding collision with
	// the Workspace lifecycle's independent event sequence.
	_, err = tx.ExecContext(ctx, `INSERT INTO thinkpixelws.outbox_messages
 (tenant_id,event_id,transaction_id,aggregate_kind,aggregate_id,aggregate_version,sequence,event_type,event_version,payload_schema,payload,occurred_at,available_at)
 VALUES ($1,$2,$3,'generation',$4,1,1,'workspace.thinkpixel.io/generation.committed.v1',1,'generation-committed.v1',$5::jsonb,$6,$6)`, g.TenantID, eventID, transactionID, g.ID, string(payload), g.CreatedAt)
	if err != nil {
		return zero, err
	}
	// Even DB work may outlast the lease. Check wall time again immediately
	// before publication; the locked head now equals this new generation.
	if err := guard.ValidateCommit(ctx, in.Writer, g.Number); err != nil {
		return zero, err
	}
	if err := validateCommitAuthority(ctx, tx, in.AuthorityExpiresAt); err != nil {
		return zero, err
	}
	if err := tx.Commit(); err != nil {
		return zero, err
	}
	return g, nil
}

var _ ports.GenerationCommitter = GenerationCommitter{}

// Database wall time is sampled after locks and again after all publication work.
func validateCommitAuthority(ctx context.Context, tx *sql.Tx, expires time.Time) error {
	var now time.Time
	if err := tx.QueryRowContext(ctx, `SELECT clock_timestamp()`).Scan(&now); err != nil {
		return err
	}
	if expires.IsZero() || !now.Before(expires) {
		return security.ErrExecutionAuthority
	}
	return nil
}
