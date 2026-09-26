package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/bdobrica/ThinkPixelWS/internal/domain"
	"github.com/bdobrica/ThinkPixelWS/internal/ports"
	"github.com/google/uuid"
)

type rowScanner interface {
	Scan(...any) error
}

type database interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

type WorkspaceRepository struct {
	db database
}

func NewWorkspaceRepository(db database) *WorkspaceRepository {
	return &WorkspaceRepository{db: db}
}

func (repository *WorkspaceRepository) Create(ctx context.Context, tenantID uuid.UUID, workspace domain.Workspace) error {
	if tenantID == uuid.Nil || tenantID != workspace.TenantID {
		return errors.New("workspace tenant scope does not match aggregate")
	}
	if err := workspace.Validate(); err != nil {
		return fmt.Errorf("validate workspace: %w", err)
	}
	residency, err := json.Marshal(append([]string{}, workspace.Residency...))
	if err != nil {
		return fmt.Errorf("encode workspace residency: %w", err)
	}
	_, err = repository.db.ExecContext(ctx, `
INSERT INTO thinkpixelws.workspaces (
    tenant_id, workspace_id, name, description, owner_kind, owner_id,
    lifecycle_state, state_version, writer_fence, classification, residency,
    created_at, updated_at
) VALUES (
    $1, $2, $3, NULLIF($4, ''), $5, $6, $7, $8, $9, $10,
    ARRAY(SELECT jsonb_array_elements_text($11::jsonb)), $12, $13
)`, tenantID, workspace.ID, workspace.Name, workspace.Description,
		workspace.Owner.Kind, workspace.Owner.ID, workspace.State, workspace.StateVersion,
		workspace.WriterFence, workspace.Classification, string(residency), workspace.CreatedAt, workspace.UpdatedAt)
	if err != nil {
		return fmt.Errorf("insert workspace: %w", err)
	}
	return nil
}

func (repository *WorkspaceRepository) Get(ctx context.Context, tenantID, workspaceID uuid.UUID) (domain.Workspace, error) {
	row := repository.db.QueryRowContext(ctx, `
SELECT tenant_id, workspace_id, name, COALESCE(description, ''), owner_kind, owner_id,
       lifecycle_state, state_version, writer_fence, classification,
       array_to_json(residency)::text, created_at, updated_at
FROM thinkpixelws.workspaces
WHERE tenant_id = $1 AND workspace_id = $2`, tenantID, workspaceID)
	workspace, err := scanWorkspace(row)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.Workspace{}, ports.ErrWorkspaceNotFound
	}
	if err != nil {
		return domain.Workspace{}, fmt.Errorf("select workspace: %w", err)
	}
	return workspace, nil
}

func (repository *WorkspaceRepository) TransitionState(
	ctx context.Context,
	tenantID, workspaceID uuid.UUID,
	current, next domain.WorkspaceState,
	expectedVersion uint64,
	transitionedAt time.Time,
) error {
	if tenantID == uuid.Nil || workspaceID == uuid.Nil {
		return errors.New("tenant and workspace IDs are required")
	}
	if expectedVersion < 1 || expectedVersion >= math.MaxInt64 {
		return errors.New("expected workspace state version is outside the supported range")
	}
	if !current.CanTransitionTo(next) {
		return fmt.Errorf("%w: %s to %s", domain.ErrInvalidWorkspaceStateTransition, current, next)
	}
	if transitionedAt.IsZero() {
		return errors.New("workspace transition time is required")
	}

	result, err := repository.db.ExecContext(ctx, `
UPDATE thinkpixelws.workspaces
SET lifecycle_state = $5,
    state_version = state_version + 1,
    updated_at = $6
WHERE tenant_id = $1
  AND workspace_id = $2
  AND lifecycle_state = $3
  AND state_version = $4
  AND updated_at <= $6`, tenantID, workspaceID, current, expectedVersion, next, transitionedAt.UTC())
	if err != nil {
		return fmt.Errorf("transition workspace state: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("read workspace transition result: %w", err)
	}
	if affected != 1 {
		return ports.ErrWorkspaceStateConflict
	}
	return nil
}

func scanWorkspace(row rowScanner) (domain.Workspace, error) {
	var workspace domain.Workspace
	var residency []byte
	err := row.Scan(
		&workspace.TenantID, &workspace.ID, &workspace.Name, &workspace.Description,
		&workspace.Owner.Kind, &workspace.Owner.ID, &workspace.State, &workspace.StateVersion,
		&workspace.WriterFence, &workspace.Classification, &residency,
		&workspace.CreatedAt, &workspace.UpdatedAt,
	)
	if err != nil {
		return domain.Workspace{}, err
	}
	if err := json.Unmarshal(residency, &workspace.Residency); err != nil {
		return domain.Workspace{}, fmt.Errorf("decode workspace residency: %w", err)
	}
	if err := workspace.Validate(); err != nil {
		return domain.Workspace{}, fmt.Errorf("validate stored workspace: %w", err)
	}
	return workspace, nil
}

var _ ports.WorkspaceRepository = (*WorkspaceRepository)(nil)
