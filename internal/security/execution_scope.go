package security

import (
	"context"
	"math"

	"github.com/bdobrica/ThinkPixelWS/internal/domain"
	"github.com/bdobrica/ThinkPixelWS/internal/ports"
	"github.com/bdobrica/ThinkPixelWS/internal/ports/clock"
	"github.com/google/uuid"
)

// ExecutionScope is the exact requested Workspace generation and per-component access.
// Callers must resolve these within the authenticated tenant and use this same
// scope for the operation. An empty component set is invalid, never a wildcard.
type ExecutionScope struct {
	WorkspaceID     uuid.UUID
	Generation      uint64
	ComponentAccess []ports.ExecutionComponentAccess
}

// VerifyExecutionScope obtains fresh verified authority and checks the requested
// scope against it. It never expands or silently trims the requested set. Success
// is only a scope/mode check: action, administrative authorization, component
// membership, classification/residency and current lease/fence remain independent.
// Read-only access accepts either grant mode; writable access requires read-write.
// Missing or unknown modes fail closed, including on unused grant components.
// All failures return zero authority and the same sanitized error.
func VerifyExecutionScope(ctx context.Context, verifier ports.ExecutionAuthorityVerifier, c clock.Clock, request ports.ExecutionAuthorityRequest, scope ExecutionScope) (ports.ExecutionAuthority, error) {
	deny := func() (ports.ExecutionAuthority, error) { return ports.ExecutionAuthority{}, ErrExecutionAuthority }
	if !authorizationUUID(scope.WorkspaceID) || scope.Generation < 1 || scope.Generation > math.MaxInt64 || len(scope.ComponentAccess) == 0 {
		return deny()
	}
	requested := make(map[uuid.UUID]domain.MaterializationMode, len(scope.ComponentAccess))
	for _, access := range scope.ComponentAccess {
		id := access.ComponentID
		if _, duplicate := requested[id]; duplicate || !authorizationUUID(id) || !validExecutionMode(access.Mode) {
			return deny()
		}
		requested[id] = access.Mode
	}
	a, err := VerifyExecutionAuthority(ctx, verifier, c, request)
	if err != nil || a.WorkspaceID != scope.WorkspaceID {
		return deny()
	}
	if a.Generation != nil && (*a.Generation < 1 || *a.Generation > math.MaxInt64 || *a.Generation != scope.Generation) {
		return deny()
	}
	allowed := make(map[uuid.UUID]domain.MaterializationMode, len(a.ComponentAccess))
	for _, access := range a.ComponentAccess {
		if _, duplicate := allowed[access.ComponentID]; duplicate || !authorizationUUID(access.ComponentID) || !validExecutionMode(access.Mode) {
			return deny()
		}
		allowed[access.ComponentID] = access.Mode
	}
	for id, mode := range requested {
		if granted, ok := allowed[id]; !ok || (mode == domain.MaterializationReadWrite && granted != domain.MaterializationReadWrite) {
			return deny()
		}
	}
	if ctx.Err() != nil {
		return deny()
	}
	return a, nil
}

func validExecutionMode(mode domain.MaterializationMode) bool {
	return mode == domain.MaterializationReadOnly || mode == domain.MaterializationReadWrite
}
