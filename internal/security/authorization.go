package security

import (
	"context"
	"errors"

	"github.com/bdobrica/ThinkPixelWS/internal/ports"
	"github.com/google/uuid"
)

// ErrForbidden exposes neither policy internals nor target existence.
var ErrForbidden = errors.New("workspace access denied")

// AuthorizeWorkspace checks one administrative operation using authenticated
// identity. The caller must resolve an existing Workspace within identity.TenantID
// and use that same tenant and target for the subsequent operation. No decisions
// are cached, and nil configuration never enables a development bypass.
func AuthorizeWorkspace(ctx context.Context, authorizer ports.WorkspaceAuthorizer, identity Identity, action ports.WorkspaceAction, workspaceID uuid.UUID) error {
	if authorizer == nil || ctx.Err() != nil || !authorizationUUID(identity.TenantID) || !identityString(identity.Principal, 256) {
		return ErrForbidden
	}
	switch action {
	case ports.WorkspaceCreate, ports.WorkspaceList:
		if workspaceID != uuid.Nil {
			return ErrForbidden
		}
	case ports.WorkspaceView, ports.WorkspaceFork, ports.WorkspaceDelete, ports.WorkspaceArchive, ports.WorkspaceRestore, ports.WorkspaceRequestMaterialization, ports.WorkspaceReadProfileMetadata:
		if !authorizationUUID(workspaceID) {
			return ErrForbidden
		}
	default:
		return ErrForbidden
	}
	decision, err := authorizer.Authorize(ctx, ports.WorkspaceAuthorizationRequest{
		TenantID: identity.TenantID, Principal: identity.Principal, Action: action, WorkspaceID: workspaceID,
	})
	if err != nil || ctx.Err() != nil || !decision.Allow {
		return ErrForbidden
	}
	return nil
}

func authorizationUUID(id uuid.UUID) bool {
	return id.Version() == 7 && id.Variant() == uuid.RFC4122
}
