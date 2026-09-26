package security

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"errors"
	"strings"
	"unicode"

	"github.com/bdobrica/ThinkPixelWS/internal/ports"
	"github.com/google/uuid"
)

// DevelopmentAuth is an explicit single-identity local policy. It never grants
// execution, materialization, profile access, or destructive operations.
// The HTTP adapter must restrict its use to loopback listeners and peers.
type DevelopmentAuth struct {
	identity  Identity
	tokenHash [32]byte
}

func NewDevelopmentAuth(identity Identity, token string) (*DevelopmentAuth, error) {
	if !authorizationUUID(identity.TenantID) || !identityString(identity.Principal, 256) {
		return nil, errors.New("development auth requires a UUIDv7 tenant and valid principal")
	}
	if len(token) < 32 || len(token) > 256 || strings.IndexFunc(token, func(r rune) bool { return r > unicode.MaxASCII || unicode.IsSpace(r) || unicode.IsControl(r) }) >= 0 {
		return nil, errors.New("development auth requires a 32-256 byte random ASCII token without whitespace")
	}
	return &DevelopmentAuth{identity: identity, tokenHash: sha256.Sum256([]byte(token))}, nil
}

func (a *DevelopmentAuth) Authenticate(ctx context.Context, token string) (Identity, error) {
	if a == nil || ctx.Err() != nil || len(token) < 32 || len(token) > 256 {
		return Identity{}, ErrUnauthenticated
	}
	hash := sha256.Sum256([]byte(token))
	if subtle.ConstantTimeCompare(hash[:], a.tokenHash[:]) != 1 {
		return Identity{}, ErrUnauthenticated
	}
	return a.identity, nil
}

func (a *DevelopmentAuth) Authorize(ctx context.Context, req ports.WorkspaceAuthorizationRequest) (ports.WorkspaceAuthorizationDecision, error) {
	if a == nil || ctx.Err() != nil || !authorizationUUID(a.identity.TenantID) || req.TenantID != a.identity.TenantID || req.Principal != a.identity.Principal {
		return ports.WorkspaceAuthorizationDecision{}, ErrForbidden
	}
	allow := false
	switch req.Action {
	case ports.WorkspaceCreate, ports.WorkspaceList:
		allow = req.WorkspaceID == uuid.Nil
	case ports.WorkspaceView:
		allow = authorizationUUID(req.WorkspaceID)
	}
	return ports.WorkspaceAuthorizationDecision{Allow: allow}, nil
}
