package materialization

import (
	"context"

	"github.com/bdobrica/ThinkPixelWS/internal/domain"
	"github.com/bdobrica/ThinkPixelWS/internal/ports"
	"github.com/bdobrica/ThinkPixelWS/internal/security"
	"github.com/google/uuid"
)

// GenerationCommitter verifies fresh execution scope after capture, before
// metadata publication. Administrative/action/policy authorization and immutable
// content verification remain prerequisites. No grant is retained or persisted.
type GenerationCommitter struct {
	Scope            RequestScopeVerifier
	Materializations ports.MaterializationRepository
	Generations      ports.GenerationCommitter
}

// Commit checks every captured component for write authority. Persistence must
// enforce exact coverage of Workspace components and recheck the deadline under
// its transaction locks. Tenant identity and capture input must be trusted.
func (c GenerationCommitter) Commit(ctx context.Context, in ports.GenerationCommit, grant string) (domain.WorkspaceGeneration, error) {
	zero := domain.WorkspaceGeneration{}
	if c.Materializations == nil || c.Generations == nil {
		return zero, security.ErrExecutionAuthority
	}
	m, err := c.Materializations.Get(ctx, in.Writer.TenantID, in.Writer.MaterializationID)
	if err != nil {
		return zero, err
	}
	if m.TenantID != in.Writer.TenantID || m.ID != in.Writer.MaterializationID || m.WorkspaceID != in.Writer.WorkspaceID || m.Mode != domain.MaterializationReadWrite || m.RunID == uuid.Nil {
		return zero, security.ErrExecutionAuthority
	}
	access := make([]ports.ExecutionComponentAccess, len(in.ComponentReferences))
	for i, ref := range in.ComponentReferences {
		access[i] = ports.ExecutionComponentAccess{ComponentID: ref.ComponentID, Mode: domain.MaterializationReadWrite}
	}
	a, err := c.Scope.Verify(ctx, Request{
		Materialization: domain.NewMaterialization{TenantID: m.TenantID, ID: m.ID, WorkspaceID: m.WorkspaceID, BaseGeneration: m.BaseGeneration, Mode: m.Mode, RunID: m.RunID, ExecutionID: m.ExecutionID},
		ComponentAccess: access, ExecutionGrant: grant,
	})
	if err != nil {
		return zero, err
	}
	if a.ExecutionID != m.ExecutionID {
		return zero, security.ErrExecutionAuthority
	}
	// Attribution and deadline derive only from verified authority.
	in.AuthorityExpiresAt, in.Principal, in.RunID = a.ExpiresAt, a.Principal, &a.RunID
	in.ExecutionID = nil
	if a.ExecutionID != uuid.Nil {
		in.ExecutionID = &a.ExecutionID
	}
	return c.Generations.Commit(ctx, in)
}
