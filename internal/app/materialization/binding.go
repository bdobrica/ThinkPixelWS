package materialization

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/bdobrica/ThinkPixelWS/internal/domain"
	"github.com/bdobrica/ThinkPixelWS/internal/ports"
	"github.com/google/uuid"
)

// Binding carries stable WS identity and versioned storage instructions for AR.
// It is not permission to attach, nor evidence that a writer lease is current.
type Binding struct {
	TenantID          uuid.UUID                   `json:"tenantId"`
	WorkspaceID       uuid.UUID                   `json:"workspaceId"`
	Generation        uint64                      `json:"generation"`
	MaterializationID uuid.UUID                   `json:"materializationId"`
	StateVersion      uint64                      `json:"stateVersion"`
	TargetID          string                      `json:"targetId"`
	AccessMode        domain.MaterializationMode  `json:"accessMode"`
	Storage           ports.WorkingStorageBinding `json:"storage"`
}

type BindingReader struct {
	Repository ports.MaterializationRepository
	Storage    ports.WorkingStorageBindingProvider
}

// Binding reads an already-authorized Materialization. No DB transaction may
// span provider I/O. A changed record invalidates the result; retry from fresh
// authority. AR must separately enforce current grants, writer leases, exclusive
// attachment/detachment and read-only mounts at the point of use.
func (r BindingReader) Binding(ctx context.Context, tenant, id uuid.UUID) (Binding, error) {
	if err := ctx.Err(); err != nil {
		return Binding{}, err
	}
	if r.Repository == nil || r.Storage == nil {
		return Binding{}, errors.New("materialization binding is not configured")
	}
	m, err := r.Repository.Get(ctx, tenant, id)
	if err != nil {
		return Binding{}, err
	}
	if m.TenantID != tenant || m.ID != id {
		return Binding{}, ports.ErrMaterializationNotFound
	}
	if err := m.Validate(); err != nil {
		return Binding{}, err
	}
	if m.Handle == "" || (m.State != domain.MaterializationReady && m.State != domain.MaterializationActive) {
		return Binding{}, ports.ErrMaterializationStateConflict
	}
	storage, observationErr := r.Storage.Binding(ctx, m)
	if err := ctx.Err(); err != nil {
		return Binding{}, err
	}
	current, err := r.Repository.Get(ctx, tenant, id)
	if err != nil {
		return Binding{}, err
	}
	if current != m {
		return Binding{}, ports.ErrMaterializationStateConflict
	}
	if observationErr != nil {
		return Binding{}, observationErr
	}
	if storage.Handle != m.Handle || storage.Kind == "" || !json.Valid(storage.Reference) {
		return Binding{}, ports.ErrWorkingStorageConflict
	}
	return Binding{TenantID: m.TenantID, WorkspaceID: m.WorkspaceID, Generation: m.BaseGeneration, MaterializationID: m.ID, StateVersion: m.StateVersion, TargetID: m.Target.ID, AccessMode: m.Mode, Storage: storage}, nil
}
