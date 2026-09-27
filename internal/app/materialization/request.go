package materialization

import (
	"context"

	"github.com/bdobrica/ThinkPixelWS/internal/domain"
	"github.com/bdobrica/ThinkPixelWS/internal/ports"
	"github.com/bdobrica/ThinkPixelWS/internal/ports/clock"
	"github.com/bdobrica/ThinkPixelWS/internal/security"
	"github.com/google/uuid"
)

// Request carries creation intent and the exact requested component access.
// Materialization.TenantID must come from authenticated identity, never the body.
// ExecutionGrant is transient: never persist it or pass it to working storage.
type Request struct {
	Materialization domain.NewMaterialization
	ComponentAccess []ports.ExecutionComponentAccess
	ExecutionGrant  string
}

type RequestScopeVerifier struct {
	Verifier ports.ExecutionAuthorityVerifier
	Clock    clock.Clock
}

// Verify checks creation intent against fresh AG authority before persistence,
// lease acquisition or storage preparation. Scope comes from the same creation
// input that will be used for the Materialization; callers cannot supply a
// separate, narrower scope for verification. Empty access is never "all".
//
// Success checks only Workspace/generation/component scope and per-component
// modes, including the overall Materialization mode. A writable Materialization
// requires write authority for every selected component, even if its requested
// component mode is read-only. Action, administrative authorization, component
// membership and policy/lease checks remain separate prerequisites.
// Callers must preserve the requested component set when persisting/preparing;
// the returned grant may contain additional components and is not that set.
func (v RequestScopeVerifier) Verify(ctx context.Context, request Request) (ports.ExecutionAuthority, error) {
	m := request.Materialization
	if m.Mode != domain.MaterializationReadOnly && m.Mode != domain.MaterializationReadWrite {
		return ports.ExecutionAuthority{}, security.ErrExecutionAuthority
	}
	authority, err := security.VerifyExecutionScope(ctx, v.Verifier, v.Clock,
		ports.ExecutionAuthorityRequest{TenantID: m.TenantID, Grant: request.ExecutionGrant},
		security.ExecutionScope{
			WorkspaceID: m.WorkspaceID, Generation: m.BaseGeneration,
			ComponentAccess: request.ComponentAccess,
		})
	if err != nil {
		return ports.ExecutionAuthority{}, err
	}
	if m.Mode == domain.MaterializationReadWrite {
		grantedModes := make(map[uuid.UUID]domain.MaterializationMode, len(authority.ComponentAccess))
		for _, access := range authority.ComponentAccess {
			grantedModes[access.ComponentID] = access.Mode
		}
		for _, access := range request.ComponentAccess {
			if grantedModes[access.ComponentID] != domain.MaterializationReadWrite {
				return ports.ExecutionAuthority{}, security.ErrExecutionAuthority
			}
		}
	}
	return authority, nil
}
