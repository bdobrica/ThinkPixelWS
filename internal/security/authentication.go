package security

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/bdobrica/ThinkPixelWS/internal/domain/shared"
	"github.com/bdobrica/ThinkPixelWS/internal/ports"
	"github.com/google/uuid"
)

// ErrUnauthenticated exposes neither credentials nor rejected claim values.
var ErrUnauthenticated = errors.New("authentication failed")

// Identity identifies a caller within the configured issuer's tenant namespace.
// It is not proof of tenant existence, membership, or Workspace authorization.
type Identity struct {
	TenantID  uuid.UUID
	Principal string
}

// ClaimMapping selects exact top-level string claims from a trusted issuer.
// Names are explicit: no aliases, nested paths, or fallback claims are used.
type ClaimMapping struct {
	TenantClaim    string
	PrincipalClaim string
}

// Authenticator verifies credentials before mapping identity. Use a verifier
// pinned to one trusted issuer; mapping must be configured by the operator.
type Authenticator struct {
	verifier ports.TokenVerifier
	mapping  ClaimMapping
}

func NewAuthenticator(verifier ports.TokenVerifier, mapping ClaimMapping) (*Authenticator, error) {
	if verifier == nil || !identityString(mapping.TenantClaim, 256) || !identityString(mapping.PrincipalClaim, 256) || mapping.TenantClaim == mapping.PrincipalClaim {
		return nil, errors.New("authentication requires a verifier and distinct tenant/principal claim names")
	}
	return &Authenticator{verifier: verifier, mapping: mapping}, nil
}

// Authenticate returns only the mapped identity, never the token or extra claims.
func (a *Authenticator) Authenticate(ctx context.Context, token string) (Identity, error) {
	if ctx.Err() != nil {
		return Identity{}, ErrUnauthenticated
	}
	claims, err := a.verifier.Verify(ctx, token)
	if err != nil || ctx.Err() != nil {
		return Identity{}, ErrUnauthenticated
	}
	var tenant, principal string
	if json.Unmarshal(claims[a.mapping.TenantClaim], &tenant) != nil || json.Unmarshal(claims[a.mapping.PrincipalClaim], &principal) != nil || !identityString(principal, 256) {
		return Identity{}, ErrUnauthenticated
	}
	id, err := shared.ParseUUIDv7(tenant)
	if err != nil {
		return Identity{}, ErrUnauthenticated
	}
	return Identity{TenantID: uuid.UUID(id), Principal: principal}, nil
}

func identityString(value string, maximum int) bool {
	return value != "" && utf8.ValidString(value) && strings.TrimSpace(value) == value && utf8.RuneCountInString(value) <= maximum && strings.IndexFunc(value, unicode.IsControl) < 0
}
