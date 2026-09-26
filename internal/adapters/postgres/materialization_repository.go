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
)

type MaterializationRepository struct{ db database }

// NewMaterializationRepository accepts either a DB or a transaction. Business
// operations use a transaction shared with their audit and outbox writes.
func NewMaterializationRepository(db database) *MaterializationRepository {
	return &MaterializationRepository{db: db}
}

func (r *MaterializationRepository) Create(ctx context.Context, tenantID uuid.UUID, m domain.Materialization) error {
	if tenantID == uuid.Nil || tenantID != m.TenantID {
		return errors.New("materialization tenant scope does not match aggregate")
	}
	if err := m.Validate(); err != nil {
		return fmt.Errorf("validate materialization: %w", err)
	}
	if m.State != domain.MaterializationRequested || m.StateVersion != 1 || !m.UpdatedAt.Equal(m.CreatedAt) {
		return errors.New("materialization must be created in its initial requested state")
	}
	_, err := r.db.ExecContext(ctx, `
INSERT INTO thinkpixelws.materializations (
 tenant_id, materialization_id, workspace_id, base_generation, provider,
 target_id, target_region, target_storage_class, target_architecture,
 mode, lifecycle_state, state_version, created_at, updated_at
) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,NULLIF($9,''),$10,$11,$12,$13,$14)`,
		m.TenantID, m.ID, m.WorkspaceID, m.BaseGeneration, m.Provider,
		m.Target.ID, m.Target.Region, m.Target.StorageClass, m.Target.Architecture,
		m.Mode, m.State, m.StateVersion, m.CreatedAt, m.UpdatedAt)
	if err != nil {
		return fmt.Errorf("insert materialization: %w", err)
	}
	return nil
}

func (r *MaterializationRepository) Get(ctx context.Context, tenantID, id uuid.UUID) (domain.Materialization, error) {
	if tenantID == uuid.Nil || id == uuid.Nil {
		return domain.Materialization{}, errors.New("tenant and materialization IDs are required")
	}
	var m domain.Materialization
	err := r.db.QueryRowContext(ctx, `
SELECT tenant_id, materialization_id, workspace_id, base_generation, provider,
 target_id, target_region, target_storage_class, COALESCE(target_architecture,''),
 mode, lifecycle_state, state_version, created_at, updated_at, COALESCE(provider_handle,''), COALESCE(clean_generation,0)
FROM thinkpixelws.materializations
WHERE tenant_id = $1 AND materialization_id = $2`, tenantID, id).Scan(
		&m.TenantID, &m.ID, &m.WorkspaceID, &m.BaseGeneration, &m.Provider,
		&m.Target.ID, &m.Target.Region, &m.Target.StorageClass, &m.Target.Architecture,
		&m.Mode, &m.State, &m.StateVersion, &m.CreatedAt, &m.UpdatedAt, &m.Handle, &m.CleanGeneration)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.Materialization{}, ports.ErrMaterializationNotFound
	}
	if err != nil {
		return domain.Materialization{}, fmt.Errorf("select materialization: %w", err)
	}
	m.CreatedAt = m.CreatedAt.UTC()
	m.UpdatedAt = m.UpdatedAt.UTC()
	if err := m.Validate(); err != nil {
		return domain.Materialization{}, fmt.Errorf("validate stored materialization: %w", err)
	}
	return m, nil
}

var _ ports.MaterializationRepository = (*MaterializationRepository)(nil)

func (r *MaterializationRepository) Bind(ctx context.Context, tenantID, id uuid.UUID, handle domain.MaterializationHandle, expectedVersion uint64, boundAt time.Time) error {
	if tenantID == uuid.Nil || id == uuid.Nil || expectedVersion < 1 || expectedVersion >= math.MaxInt64 || boundAt.IsZero() {
		return errors.New("invalid materialization binding scope, version or time")
	}
	if err := handle.Validate(); err != nil {
		return err
	}
	result, err := r.db.ExecContext(ctx, `
UPDATE thinkpixelws.materializations
SET provider_handle = $3, state_version = state_version + 1, updated_at = $5
WHERE tenant_id = $1 AND materialization_id = $2
  AND lifecycle_state = 'PREPARING' AND provider_handle IS NULL
  AND state_version = $4 AND updated_at <= $5`, tenantID, id, handle, expectedVersion, boundAt.UTC())
	if err != nil {
		return fmt.Errorf("bind materialization: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("read materialization binding result: %w", err)
	}
	if affected != 1 {
		return ports.ErrMaterializationStateConflict
	}
	return nil
}

// TransitionState atomically checks the observed tenant, state, version and time.
// It accepts a shared transaction so business mutations can include audit/outbox.
func (r *MaterializationRepository) TransitionState(ctx context.Context, tenantID, id uuid.UUID, current, next domain.MaterializationState, expectedVersion uint64, transitionedAt time.Time) error {
	if tenantID == uuid.Nil || id == uuid.Nil {
		return errors.New("tenant and materialization IDs are required")
	}
	if expectedVersion < 1 || expectedVersion >= math.MaxInt64 {
		return errors.New("expected materialization state version is outside the supported range")
	}
	if !current.CanTransitionTo(next) {
		return fmt.Errorf("%w: %s to %s", domain.ErrInvalidMaterializationStateTransition, current, next)
	}
	if transitionedAt.IsZero() {
		return errors.New("materialization transition time is required")
	}
	result, err := r.db.ExecContext(ctx, `
UPDATE thinkpixelws.materializations
SET lifecycle_state = $5, clean_generation = NULL, state_version = state_version + 1, updated_at = $6
WHERE tenant_id = $1 AND materialization_id = $2
  AND lifecycle_state = $3 AND state_version = $4 AND updated_at <= $6`,
		tenantID, id, current, expectedVersion, next, transitionedAt.UTC())
	if err != nil {
		return fmt.Errorf("transition materialization state: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("read materialization transition result: %w", err)
	}
	if affected != 1 {
		return ports.ErrMaterializationStateConflict
	}
	return nil
}
