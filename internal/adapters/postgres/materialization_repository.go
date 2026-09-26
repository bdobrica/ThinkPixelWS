package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

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
 mode, lifecycle_state, state_version, created_at, updated_at
FROM thinkpixelws.materializations
WHERE tenant_id = $1 AND materialization_id = $2`, tenantID, id).Scan(
		&m.TenantID, &m.ID, &m.WorkspaceID, &m.BaseGeneration, &m.Provider,
		&m.Target.ID, &m.Target.Region, &m.Target.StorageClass, &m.Target.Architecture,
		&m.Mode, &m.State, &m.StateVersion, &m.CreatedAt, &m.UpdatedAt)
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
