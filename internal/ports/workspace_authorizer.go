package ports

import (
	"context"

	"github.com/google/uuid"
)

// WorkspaceAction names an administrative permission, never an execution grant.
type WorkspaceAction string

const (
	WorkspaceCreate                 WorkspaceAction = "workspace.create"
	WorkspaceList                   WorkspaceAction = "workspace.list"
	WorkspaceView                   WorkspaceAction = "workspace.view"
	WorkspaceFork                   WorkspaceAction = "workspace.fork"
	WorkspaceDelete                 WorkspaceAction = "workspace.delete"
	WorkspaceArchive                WorkspaceAction = "workspace.archive"
	WorkspaceRestore                WorkspaceAction = "workspace.restore"
	WorkspaceRequestMaterialization WorkspaceAction = "workspace.request_materialization"
	WorkspaceReadProfileMetadata    WorkspaceAction = "workspace.read_profile_metadata"
)

// WorkspaceAuthorizationRequest carries trusted identity and the exact target.
// TenantID scopes both caller and target; cross-tenant administration is not
// supported. WorkspaceID is nil only for tenant-scoped create/list operations.
// Callers must obtain identity from authentication and resolve existing targets
// within that tenant, not accept tenant or principal overrides from request data.
type WorkspaceAuthorizationRequest struct {
	TenantID    uuid.UUID
	Principal   string
	Action      WorkspaceAction
	WorkspaceID uuid.UUID
}

// WorkspaceAuthorizationDecision defaults to deny. Allow is valid only when
// the authorizer also returns a nil error and the request context remains live.
type WorkspaceAuthorizationDecision struct {
	Allow bool
}

// WorkspaceAuthorizer evaluates administrative policy for each operation.
// Implementations must honor context cancellation and explicitly deny absent
// policy. Errors (including policy unavailability) must never grant access.
// Membership, bindings, and this decision confer no Run or external-tool authority.
// Integrated execution additionally requires independent AG grant verification.
type WorkspaceAuthorizer interface {
	Authorize(context.Context, WorkspaceAuthorizationRequest) (WorkspaceAuthorizationDecision, error)
}
