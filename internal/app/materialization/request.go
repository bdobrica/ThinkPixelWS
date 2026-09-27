package materialization

import (
	"context"

	"github.com/bdobrica/ThinkPixelWS/internal/domain"
	"github.com/bdobrica/ThinkPixelWS/internal/ports"
	"github.com/bdobrica/ThinkPixelWS/internal/ports/clock"
	"github.com/bdobrica/ThinkPixelWS/internal/security"
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
// modes. Overall Materialization mode, action, administrative authorization,
// component membership and policy/lease checks remain separate prerequisites.
// Callers must preserve the requested component set when persisting/preparing;
// the returned grant may contain additional components and is not that set.
func (v RequestScopeVerifier) Verify(ctx context.Context, request Request) (ports.ExecutionAuthority, error) {
	m := request.Materialization
	return security.VerifyExecutionScope(ctx, v.Verifier, v.Clock,
		ports.ExecutionAuthorityRequest{TenantID: m.TenantID, Grant: request.ExecutionGrant},
		security.ExecutionScope{
			WorkspaceID: m.WorkspaceID, Generation: m.BaseGeneration,
			ComponentAccess: request.ComponentAccess,
		})
}
