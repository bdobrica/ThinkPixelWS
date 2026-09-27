package materialization

import (
	"context"

	"github.com/bdobrica/ThinkPixelWS/internal/domain"
	"github.com/bdobrica/ThinkPixelWS/internal/ports"
	"github.com/bdobrica/ThinkPixelWS/internal/security"
	"github.com/google/uuid"
)

// LeaseRenewer checks fresh execution scope before each persistence renewal.
// Administrative/action authorization and policy checks remain prerequisites.
// No credential or verified authority is retained between calls.
type LeaseRenewer struct {
	Scope            RequestScopeVerifier
	Materializations ports.MaterializationRepository
	Leases           ports.MaterializationLeaseRepository
}

// Renew loads the tenant-scoped record and binds verification to its immutable
// Workspace, generation, Run and Execution. access must be the complete component
// scope resolved by trusted application code for that Materialization, never a
// caller-selected subset. Component-scope persistence and HTTP wiring are pending.
func (r LeaseRenewer) Renew(ctx context.Context, tenantID, materializationID, leaseID uuid.UUID, fence uint64, holder, grant string, access []ports.ExecutionComponentAccess) (domain.MaterializationLease, error) {
	empty := domain.MaterializationLease{}
	if r.Materializations == nil || r.Leases == nil {
		return empty, security.ErrExecutionAuthority
	}
	m, err := r.Materializations.Get(ctx, tenantID, materializationID)
	if err != nil {
		return empty, err
	}
	if m.TenantID != tenantID || m.ID != materializationID || m.Mode != domain.MaterializationReadWrite || m.RunID == uuid.Nil {
		return empty, security.ErrExecutionAuthority
	}
	authority, err := r.Scope.Verify(ctx, Request{
		Materialization: domain.NewMaterialization{TenantID: tenantID, ID: m.ID, WorkspaceID: m.WorkspaceID, BaseGeneration: m.BaseGeneration, Mode: m.Mode, RunID: m.RunID, ExecutionID: m.ExecutionID},
		ComponentAccess: access, ExecutionGrant: grant,
	})
	if err != nil {
		return empty, err
	}
	// Optional Execution must match exactly, including Run-only records.
	if authority.ExecutionID != m.ExecutionID {
		return empty, security.ErrExecutionAuthority
	}
	return r.Leases.Renew(ctx, tenantID, m.ID, leaseID, fence, holder, authority.ExpiresAt)
}
