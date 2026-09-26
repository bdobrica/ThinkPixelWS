package security

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/bdobrica/ThinkPixelWS/internal/ports"
)

type verifierFunc func(context.Context, string) (ports.TokenClaims, error)

func (f verifierFunc) Verify(ctx context.Context, token string) (ports.TokenClaims, error) {
	return f(ctx, token)
}

func TestAuthenticateMapping(t *testing.T) {
	const tenant = "01900000-0000-7000-8000-000000000001"
	for _, tc := range []struct {
		name, tenant, principal string
		valid                   bool
	}{
		{"valid", `"` + tenant + `"`, `"subject-1"`, true},
		{"unicode principal boundary", `"` + tenant + `"`, `"` + strings.Repeat("é", 256) + `"`, true},
		{"missing tenant", "", `"subject-1"`, false},
		{"null tenant", "null", `"subject-1"`, false},
		{"tenant list", `["` + tenant + `"]`, `"subject-1"`, false},
		{"tenant whitespace", `" ` + tenant + `"`, `"subject-1"`, false},
		{"tenant v4", `"01900000-0000-4000-8000-000000000001"`, `"subject-1"`, false},
		{"missing principal", `"` + tenant + `"`, "", false},
		{"null principal", `"` + tenant + `"`, "null", false},
		{"empty principal", `"` + tenant + `"`, `""`, false},
		{"numeric principal", `"` + tenant + `"`, "123", false},
		{"principal object", `"` + tenant + `"`, `{}`, false},
		{"principal list", `"` + tenant + `"`, `["subject-1"]`, false},
		{"principal whitespace", `"` + tenant + `"`, `" subject-1"`, false},
		{"principal control", `"` + tenant + `"`, `"subject\n1"`, false},
		{"principal too long", `"` + tenant + `"`, `"` + strings.Repeat("x", 257) + `"`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			claims := ports.TokenClaims{"org": json.RawMessage(tc.tenant), "actor": json.RawMessage(tc.principal), "tenant_id": json.RawMessage(`"` + tenant + `"`), "sub": json.RawMessage(`"fallback"`), "roles": json.RawMessage(`["admin"]`)}
			a, err := NewAuthenticator(verifierFunc(func(_ context.Context, token string) (ports.TokenClaims, error) {
				if token != "credential" {
					t.Fatal("credential was not passed to verifier")
				}
				return claims, nil
			}), ClaimMapping{TenantClaim: "org", PrincipalClaim: "actor"})
			if err != nil {
				t.Fatal(err)
			}
			got, err := a.Authenticate(context.Background(), "credential")
			if !tc.valid {
				if err != ErrUnauthenticated || got != (Identity{}) {
					t.Fatalf("got %v, %v", got, err)
				}
				return
			}
			var principal string
			_ = json.Unmarshal([]byte(tc.principal), &principal)
			if err != nil || got.TenantID.String() != tenant || got.Principal != principal {
				t.Fatalf("got %v, %v", got, err)
			}
		})
	}
}

func TestAuthenticateRejectsVerificationFailureAndCancellation(t *testing.T) {
	for _, cancelBefore := range []bool{false, true} {
		ctx, cancel := context.WithCancel(context.Background())
		calls := 0
		a, err := NewAuthenticator(verifierFunc(func(context.Context, string) (ports.TokenClaims, error) {
			calls++
			return nil, errors.New("sensitive upstream error")
		}), ClaimMapping{"tenant_id", "sub"})
		if err != nil {
			t.Fatal(err)
		}
		if cancelBefore {
			cancel()
		}
		got, err := a.Authenticate(ctx, "secret")
		cancel()
		if err != ErrUnauthenticated || got != (Identity{}) || (cancelBefore && calls != 0) {
			t.Fatalf("got %v, %v; calls %d", got, err, calls)
		}
	}
}

func TestAuthenticatorConfiguration(t *testing.T) {
	v := verifierFunc(func(context.Context, string) (ports.TokenClaims, error) { return nil, nil })
	for _, mapping := range []ClaimMapping{{}, {"tenant", ""}, {"", "sub"}, {"same", "same"}, {" tenant", "sub"}, {"tenant", "sub\n"}} {
		if _, err := NewAuthenticator(v, mapping); err == nil {
			t.Fatalf("accepted mapping %#v", mapping)
		}
	}
	if _, err := NewAuthenticator(nil, ClaimMapping{"tenant", "sub"}); err == nil {
		t.Fatal("accepted nil verifier")
	}
}
