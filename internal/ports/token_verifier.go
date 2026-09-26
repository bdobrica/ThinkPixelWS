package ports

import (
	"context"
	"encoding/json"
)

// TokenClaims are authenticated issuer claims, not tenant membership or authority.
// Claim mapping and authorization must be performed separately.
type TokenClaims map[string]json.RawMessage

// TokenVerifier validates a raw compact JWT without retaining the credential.
type TokenVerifier interface {
	Verify(context.Context, string) (TokenClaims, error)
}
