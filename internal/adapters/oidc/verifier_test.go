package oidc

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"errors"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/bdobrica/ThinkPixelWS/internal/security"
	"github.com/golang-jwt/jwt/v5"
)

func TestVerifiedIdentityMapping(t *testing.T) {
	f := newFixture(t)
	a, err := security.NewAuthenticator(f.verifier, security.ClaimMapping{TenantClaim: "tenant_id", PrincipalClaim: "sub"})
	if err != nil {
		t.Fatal(err)
	}
	claims := f.claims()
	claims["tenant_id"] = "01900000-0000-7000-8000-000000000001"
	raw := sign(t, claims, f.kid, jwt.SigningMethodRS256, f.private)
	identity, err := a.Authenticate(context.Background(), raw)
	if err != nil || identity.TenantID.String() != claims["tenant_id"] || identity.Principal != claims["sub"] {
		t.Fatalf("mapped identity: %v, %v", identity, err)
	}
	// A trusted signature alone is insufficient when identity claims are missing.
	delete(claims, "tenant_id")
	raw = sign(t, claims, f.kid, jwt.SigningMethodRS256, f.private)
	if got, err := a.Authenticate(context.Background(), raw); err != security.ErrUnauthenticated || got != (security.Identity{}) {
		t.Fatalf("accepted missing tenant: %v, %v", got, err)
	}
	claims["tenant_id"] = "01900000-0000-7000-8000-000000000001"
	claims["iss"] = "https://untrusted.example"
	raw = sign(t, claims, f.kid, jwt.SigningMethodRS256, f.private)
	if got, err := a.Authenticate(context.Background(), raw); err != security.ErrUnauthenticated || got != (security.Identity{}) {
		t.Fatalf("accepted untrusted issuer: %v, %v", got, err)
	}
}

type testClock struct{ seconds atomic.Int64 }

func (c *testClock) Now() time.Time { return time.Unix(c.seconds.Load(), 0) }

type fixture struct {
	server         *httptest.Server
	clock          *testClock
	verifier       *Verifier
	private        *rsa.PrivateKey
	mu             sync.Mutex
	kid            string
	failed         bool
	metadataIssuer string
	jwksURI        string
	jwks           any
	requests       int
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	private, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	f := &fixture{private: private, clock: &testClock{}, kid: "first"}
	f.clock.seconds.Store(1800000000)
	f.server = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		f.requests++
		if f.failed {
			http.Error(w, "unavailable", 503)
			return
		}
		if strings.HasSuffix(r.URL.Path, "openid-configuration") {
			issuer := f.server.URL
			if f.metadataIssuer != "" {
				issuer = f.metadataIssuer
			}
			uri := f.server.URL + "/keys"
			if f.jwksURI != "" {
				uri = f.jwksURI
			}
			_ = json.NewEncoder(w).Encode(map[string]string{"issuer": issuer, "jwks_uri": uri})
		} else {
			if f.jwks != nil {
				_ = json.NewEncoder(w).Encode(f.jwks)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"keys": []any{map[string]string{
				"kty": "RSA", "kid": f.kid, "use": "sig", "alg": "RS256",
				"n": base64.RawURLEncoding.EncodeToString(f.private.N.Bytes()),
				"e": base64.RawURLEncoding.EncodeToString(big.NewInt(int64(f.private.E)).Bytes()),
			}}})
		}
	}))
	t.Cleanup(f.server.Close)
	f.verifier, err = New(Config{Issuer: f.server.URL, Audience: "workspace-api", Client: f.server.Client(), Clock: f.clock})
	if err != nil {
		t.Fatal(err)
	}
	return f
}

func (f *fixture) claims() jwt.MapClaims {
	return jwt.MapClaims{"iss": f.server.URL, "aud": "workspace-api", "exp": f.clock.Now().Add(time.Minute).Unix(), "nbf": f.clock.Now().Unix(), "sub": "principal", "tenant": "example"}
}
func sign(t *testing.T, claims jwt.MapClaims, kid string, method jwt.SigningMethod, key any) string {
	t.Helper()
	token := jwt.NewWithClaims(method, claims)
	if kid != "" {
		token.Header["kid"] = kid
	}
	raw, err := token.SignedString(key)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func TestVerifyClaimsAndSignature(t *testing.T) {
	f := newFixture(t)
	other, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name   string
		change func(jwt.MapClaims)
		kid    string
		method jwt.SigningMethod
		key    any
		valid  bool
	}{
		{name: "valid", valid: true},
		{name: "audience list", change: func(c jwt.MapClaims) { c["aud"] = []string{"another", "workspace-api"} }, valid: true},
		{name: "optional nbf", change: func(c jwt.MapClaims) { delete(c, "nbf") }, valid: true},
		{name: "issuer", change: func(c jwt.MapClaims) { c["iss"] = "https://attacker.invalid" }},
		{name: "missing issuer", change: func(c jwt.MapClaims) { delete(c, "iss") }},
		{name: "audience", change: func(c jwt.MapClaims) { c["aud"] = "another" }},
		{name: "missing audience", change: func(c jwt.MapClaims) { delete(c, "aud") }},
		{name: "missing expiry", change: func(c jwt.MapClaims) { delete(c, "exp") }},
		{name: "null expiry", change: func(c jwt.MapClaims) { c["exp"] = nil }},
		{name: "string expiry", change: func(c jwt.MapClaims) { c["exp"] = "1800000060" }},
		{name: "expired", change: func(c jwt.MapClaims) { c["exp"] = f.clock.Now().Unix() - 1 }},
		{name: "expiry boundary", change: func(c jwt.MapClaims) { c["exp"] = f.clock.Now().Unix() }},
		{name: "future nbf", change: func(c jwt.MapClaims) { c["nbf"] = f.clock.Now().Unix() + 1 }},
		{name: "invalid nbf", change: func(c jwt.MapClaims) { c["nbf"] = "tomorrow" }},
		{name: "unknown kid", kid: "unknown"},
		{name: "wrong signature", key: other},
		{name: "other asymmetric algorithm", method: jwt.SigningMethodRS512},
		{name: "HMAC confusion", method: jwt.SigningMethodHS256, key: []byte("not-a-trusted-signing-key")},
		{name: "unsigned", method: jwt.SigningMethodNone, key: jwt.UnsafeAllowNoneSignatureType},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			claims := f.claims()
			if tc.change != nil {
				tc.change(claims)
			}
			kid := tc.kid
			if kid == "" {
				kid = "first"
			}
			method := tc.method
			if method == nil {
				method = jwt.SigningMethodRS256
			}
			key := tc.key
			if key == nil {
				key = f.private
			}
			raw := sign(t, claims, kid, method, key)
			got, err := f.verifier.Verify(context.Background(), raw)
			if tc.valid {
				if err != nil || string(got["tenant"]) != `"example"` {
					t.Fatalf("valid token: claims=%v err=%v", got, err)
				}
			} else if !errors.Is(err, ErrInvalidToken) || got != nil {
				t.Fatalf("accepted invalid token: %v", err)
			}
		})
	}
	for _, raw := range []string{"", "broken.token", strings.Repeat("a", maxTokenBytes+1), sign(t, f.claims(), "", jwt.SigningMethodRS256, f.private)} {
		if got, err := f.verifier.Verify(context.Background(), raw); err != ErrInvalidToken || got != nil {
			t.Fatal("malformed token accepted")
		}
	}
}

func TestRotationAndCacheExpiry(t *testing.T) {
	f := newFixture(t)
	verify := func(kid string, want bool) {
		t.Helper()
		_, err := f.verifier.Verify(context.Background(), sign(t, f.claims(), kid, jwt.SigningMethodRS256, f.private))
		if (err == nil) != want {
			t.Fatalf("kid %s: %v", kid, err)
		}
	}
	verify("first", true)
	f.mu.Lock()
	requests := f.requests
	f.mu.Unlock()
	verify("first", true)
	f.mu.Lock()
	if f.requests != requests {
		t.Error("cached key refetched")
	}
	f.kid = "second"
	f.mu.Unlock()
	f.clock.seconds.Add(5)
	verify("second", true)
	verify("first", false) // Replacement key set removes retired keys.
	f.mu.Lock()
	f.failed = true
	f.mu.Unlock()
	verify("second", true) // Fresh cached key remains usable during outage.
	f.clock.seconds.Add(300)
	verify("second", false) // Expired cache cannot bypass failed refresh.
	f.mu.Lock()
	f.failed = false
	f.mu.Unlock()
	f.clock.seconds.Add(5)
	verify("second", true)
}

func TestDiscoveryFailsClosed(t *testing.T) {
	for _, tc := range []string{"issuer mismatch", "insecure jwks", "unavailable", "invalid jwks"} {
		t.Run(tc, func(t *testing.T) {
			f := newFixture(t)
			f.mu.Lock()
			switch tc {
			case "issuer mismatch":
				f.metadataIssuer = "https://attacker.invalid"
			case "insecure jwks":
				f.jwksURI = "http://attacker.invalid/keys"
			case "unavailable":
				f.failed = true
			case "invalid jwks":
				f.jwks = map[string]any{"keys": []any{}}
			}
			f.mu.Unlock()
			raw := sign(t, f.claims(), "first", jwt.SigningMethodRS256, f.private)
			if _, err := f.verifier.Verify(context.Background(), raw); err != ErrInvalidToken {
				t.Fatalf("discovery accepted: %v", err)
			}
		})
	}
}

func TestConcurrentVerificationAndCancellation(t *testing.T) {
	f := newFixture(t)
	raw := sign(t, f.claims(), "first", jwt.SigningMethodRS256, f.private)
	var wg sync.WaitGroup
	for range 16 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := f.verifier.Verify(context.Background(), raw); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	f.mu.Lock()
	if f.requests != 2 {
		t.Errorf("expected one discovery and JWKS fetch, got %d", f.requests)
	}
	f.mu.Unlock()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := f.verifier.Verify(ctx, raw); err != ErrInvalidToken {
		t.Fatal("cancelled verification accepted")
	}
}

func TestInvalidConfiguration(t *testing.T) {
	for _, issuer := range []string{"", "http://issuer.invalid", "https://user:pass@issuer.invalid", "https://issuer.invalid?query=x", "https://issuer.invalid#fragment"} {
		if _, err := New(Config{Issuer: issuer, Audience: "api", Clock: &testClock{}}); err == nil {
			t.Errorf("accepted issuer %q", issuer)
		}
	}
	if _, err := New(Config{Issuer: "https://issuer.invalid", Clock: &testClock{}}); err == nil {
		t.Fatal("accepted missing audience")
	}
	if _, err := New(Config{Issuer: "https://issuer.invalid", Audience: "api"}); err == nil {
		t.Fatal("accepted missing clock")
	}
}
