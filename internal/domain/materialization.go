package domain

import (
	"errors"
	"math"
	"time"

	"github.com/google/uuid"
)

type MaterializationMode string

const (
	MaterializationReadOnly  MaterializationMode = "read-only"
	MaterializationReadWrite MaterializationMode = "read-write"
)

type MaterializationState string

const (
	MaterializationRequested     MaterializationState = "REQUESTED"
	MaterializationPreparing     MaterializationState = "PREPARING"
	MaterializationReady         MaterializationState = "READY"
	MaterializationActive        MaterializationState = "ACTIVE"
	MaterializationCheckpointing MaterializationState = "CHECKPOINTING"
	MaterializationReleasing     MaterializationState = "RELEASING"
	MaterializationReleased      MaterializationState = "RELEASED"
	MaterializationFailed        MaterializationState = "FAILED"
	MaterializationFenced        MaterializationState = "FENCED"
)

// MaterializationTarget describes requested placement, not permission to use it.
// Provider binding handles and credentials are deliberately separate.
type MaterializationTarget struct {
	ID           string
	Region       string
	StorageClass string
	Architecture string
}

// Materialization records a temporary realization of one completed generation.
// A read-write record is only intent: it does not establish a writer lease,
// execution authority, or permission to provision or attach storage.
type Materialization struct {
	TenantID       uuid.UUID
	ID             uuid.UUID
	WorkspaceID    uuid.UUID
	BaseGeneration uint64
	Provider       string
	Target         MaterializationTarget
	Mode           MaterializationMode
	State          MaterializationState
	StateVersion   uint64
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

type NewMaterialization struct {
	TenantID       uuid.UUID
	ID             uuid.UUID
	WorkspaceID    uuid.UUID
	BaseGeneration uint64
	Provider       string
	Target         MaterializationTarget
	Mode           MaterializationMode
}

func (input NewMaterialization) Materialization(now time.Time) (Materialization, error) {
	m := Materialization{
		TenantID: input.TenantID, ID: input.ID, WorkspaceID: input.WorkspaceID,
		BaseGeneration: input.BaseGeneration, Provider: input.Provider, Target: input.Target,
		Mode: input.Mode, State: MaterializationRequested, StateVersion: 1,
		CreatedAt: now.UTC(), UpdatedAt: now.UTC(),
	}
	if err := m.Validate(); err != nil {
		return Materialization{}, err
	}
	return m, nil
}

func (m Materialization) Validate() error {
	for _, id := range []uuid.UUID{m.TenantID, m.ID, m.WorkspaceID} {
		if id.Version() != 7 || id.Variant() != uuid.RFC4122 {
			return errors.New("materialization IDs must be UUIDv7")
		}
	}
	if m.BaseGeneration < 1 || m.BaseGeneration > math.MaxInt64 {
		return errors.New("materialization base generation is invalid")
	}
	if !sourceProviderPattern.MatchString(m.Provider) {
		return errors.New("materialization provider is invalid")
	}
	if !validBoundedSourceValue(m.Target.ID, 256) || !validBoundedSourceValue(m.Target.Region, 63) || !validBoundedSourceValue(m.Target.StorageClass, 253) ||
		(m.Target.Architecture != "" && !validBoundedSourceValue(m.Target.Architecture, 32)) {
		return errors.New("materialization target is invalid")
	}
	if m.Mode != MaterializationReadOnly && m.Mode != MaterializationReadWrite {
		return errors.New("materialization mode is invalid")
	}
	switch m.State {
	case MaterializationRequested, MaterializationPreparing, MaterializationReady, MaterializationActive,
		MaterializationCheckpointing, MaterializationReleasing, MaterializationReleased, MaterializationFailed, MaterializationFenced:
	default:
		return errors.New("materialization state is invalid")
	}
	if m.StateVersion < 1 || m.StateVersion > math.MaxInt64 {
		return errors.New("materialization state version is invalid")
	}
	if m.CreatedAt.IsZero() || m.UpdatedAt.Before(m.CreatedAt) {
		return errors.New("materialization timestamps are invalid")
	}
	return nil
}
