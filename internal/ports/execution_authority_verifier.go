package ports

import (
	"context"
	"time"

	"github.com/bdobrica/ThinkPixelWS/internal/domain"
	"github.com/google/uuid"
)

const ExecutionAuthorityAudience = "thinkpixelws"

// ExecutionAuthorityRequest carries an opaque credential and the authenticated
// caller's tenant. Grant must never be logged, persisted, or returned to callers.
// Caller authentication is independent of grant verification.
type ExecutionAuthorityRequest struct {
	TenantID uuid.UUID
	Grant    string
}

type ExecutionComponentAccess struct {
	ComponentID uuid.UUID
	Mode        domain.MaterializationMode
}

// ExecutionAuthority is verified AG authority, not a Workspace operation
// decision. Consumers must intersect its scope with the requested operation,
// administrative policy, storage scope and current WS lease/fence.
// A nil Generation permits any generation within the remaining grant bounds;
// a non-nil value pins an exact generation. ExecutionID may be absent.
type ExecutionAuthority struct {
	Issuer                string
	Audience              string
	GrantID               string
	TenantID              uuid.UUID
	Principal             string
	RunID                 uuid.UUID
	ExecutionID           uuid.UUID
	WorkspaceID           uuid.UUID
	Generation            *uint64
	ComponentAccess       []ExecutionComponentAccess
	Actions               []string
	ClassificationCeiling domain.Classification
	ResidencyConstraints  []string
	IssuedAt              time.Time
	NotBefore             time.Time
	ExpiresAt             time.Time
}

// ExecutionAuthorityVerifier is WS's transport-independent boundary to AG.
// Implementations must authenticate the configured issuer, verify integrity and
// audience thinkpixelws, validate all required claims and time bounds, and check
// current revocation/cancellation under the agreed AG freshness contract on every
// call. Unavailable or uncertain authority must return an error and no authority.
// Decoding a token, authenticating a service, or finding Workspace membership is
// insufficient. Implementations must honor cancellation and never retain grants
// or include them in errors. No AG transport or internal types cross this port.
type ExecutionAuthorityVerifier interface {
	VerifyExecutionAuthority(context.Context, ExecutionAuthorityRequest) (ExecutionAuthority, error)
}
