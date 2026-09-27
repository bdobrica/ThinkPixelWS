package security

import (
	"context"
	"errors"
	"strings"

	"github.com/bdobrica/ThinkPixelWS/internal/ports"
	"github.com/bdobrica/ThinkPixelWS/internal/ports/clock"
)

var ErrExecutionAuthority = errors.New("execution authority unavailable or denied")

// VerifyExecutionAuthority obtains fresh authority without caching. Successful
// verification alone never authorizes a Materialization or other WS operation:
// callers must still enforce Workspace/generation/component/action/mode scope.
// Errors expose no credential or verifier details and always return zero authority.
func VerifyExecutionAuthority(ctx context.Context, verifier ports.ExecutionAuthorityVerifier, c clock.Clock, request ports.ExecutionAuthorityRequest) (ports.ExecutionAuthority, error) {
	deny := func() (ports.ExecutionAuthority, error) { return ports.ExecutionAuthority{}, ErrExecutionAuthority }
	if verifier == nil || c == nil || ctx.Err() != nil || !authorizationUUID(request.TenantID) || strings.TrimSpace(request.Grant) == "" {
		return deny()
	}
	a, err := verifier.VerifyExecutionAuthority(ctx, request)
	if err != nil || ctx.Err() != nil {
		return deny()
	}
	now := c.Now()
	if a.TenantID != request.TenantID || a.Audience != ports.ExecutionAuthorityAudience ||
		!identityString(a.Issuer, 2048) || !identityString(a.GrantID, 256) || !identityString(a.Principal, 256) ||
		!authorizationUUID(a.RunID) || !authorizationUUID(a.WorkspaceID) ||
		a.IssuedAt.IsZero() || a.NotBefore.IsZero() || a.ExpiresAt.IsZero() ||
		a.IssuedAt.After(now) || a.NotBefore.After(now) || !now.Before(a.ExpiresAt) {
		return deny()
	}
	return a, nil
}
