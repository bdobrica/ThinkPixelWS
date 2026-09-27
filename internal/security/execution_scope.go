package security

import (
	"context"
	"math"

	"github.com/bdobrica/ThinkPixelWS/internal/ports"
	"github.com/bdobrica/ThinkPixelWS/internal/ports/clock"
	"github.com/google/uuid"
)

// ExecutionScope is the exact requested Workspace generation and component set.
// Callers must resolve these within the authenticated tenant and use this same
// scope for the operation. An empty component set is invalid, never a wildcard.
type ExecutionScope struct {
	WorkspaceID  uuid.UUID
	Generation   uint64
	ComponentIDs []uuid.UUID
}

// VerifyExecutionScope obtains fresh verified authority and checks the requested
// scope against it. It never expands or silently trims the requested set. Success
// is only a scope check: mode, action, administrative authorization, component
// membership, classification/residency and current lease/fence remain independent.
// All failures return zero authority and the same sanitized error.
func VerifyExecutionScope(ctx context.Context, verifier ports.ExecutionAuthorityVerifier, c clock.Clock, request ports.ExecutionAuthorityRequest, scope ExecutionScope) (ports.ExecutionAuthority, error) {
	deny := func() (ports.ExecutionAuthority, error) { return ports.ExecutionAuthority{}, ErrExecutionAuthority }
	if !authorizationUUID(scope.WorkspaceID) || scope.Generation < 1 || scope.Generation > math.MaxInt64 || len(scope.ComponentIDs) == 0 {
		return deny()
	}
	requested := make(map[uuid.UUID]struct{}, len(scope.ComponentIDs))
	for _, id := range scope.ComponentIDs {
		if _, duplicate := requested[id]; duplicate || !authorizationUUID(id) {
			return deny()
		}
		requested[id] = struct{}{}
	}
	a, err := VerifyExecutionAuthority(ctx, verifier, c, request)
	if err != nil || a.WorkspaceID != scope.WorkspaceID {
		return deny()
	}
	if a.Generation != nil && (*a.Generation < 1 || *a.Generation > math.MaxInt64 || *a.Generation != scope.Generation) {
		return deny()
	}
	allowed := make(map[uuid.UUID]struct{}, len(a.ComponentAccess))
	for _, access := range a.ComponentAccess {
		if _, duplicate := allowed[access.ComponentID]; duplicate || !authorizationUUID(access.ComponentID) {
			return deny()
		}
		allowed[access.ComponentID] = struct{}{}
	}
	for id := range requested {
		if _, ok := allowed[id]; !ok {
			return deny()
		}
	}
	if ctx.Err() != nil {
		return deny()
	}
	return a, nil
}
