// Package oidc validates bearer JWTs using keys discovered from a pinned issuer.
package oidc

import (
	"context"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"math/big"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/bdobrica/ThinkPixelWS/internal/ports"
	clockport "github.com/bdobrica/ThinkPixelWS/internal/ports/clock"
	"github.com/golang-jwt/jwt/v5"
)

const (
	maxTokenBytes    = 16 << 10
	maxDocumentBytes = 1 << 20
	keyLifetime      = 5 * time.Minute
	refreshInterval  = 5 * time.Second
)

// ErrInvalidToken deliberately contains no token, claims, or upstream response.
var ErrInvalidToken = errors.New("invalid bearer token")

// Config pins the trusted issuer and the audience of Workspace API tokens.
// This initial adapter accepts RS256 only, regardless of issuer metadata.
// Client may supply a trusted transport (e.g. private CA); redirects are disabled
// and the adapter caps all HTTP requests at five seconds.
type Config struct {
	Issuer   string
	Audience string
	Client   *http.Client
	Clock    clockport.Clock
}

type Verifier struct {
	issuer, audience     string
	client               *http.Client
	clock                clockport.Clock
	mu                   sync.Mutex
	keys                 map[string]*rsa.PublicKey
	expires, nextRefresh time.Time
}

var _ ports.TokenVerifier = (*Verifier)(nil)

// New validates configuration. Discovery is lazy and uses the caller's context.
func New(cfg Config) (*Verifier, error) {
	if !validURL(cfg.Issuer) || strings.TrimSpace(cfg.Audience) == "" || cfg.Clock == nil {
		return nil, errors.New("OIDC requires an HTTPS issuer, audience, and clock")
	}
	client := http.Client{}
	if cfg.Client != nil {
		client = *cfg.Client
	}
	if client.Timeout <= 0 || client.Timeout > 5*time.Second {
		client.Timeout = 5 * time.Second
	}
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return errors.New("OIDC redirects are disabled") }
	return &Verifier{issuer: cfg.Issuer, audience: cfg.Audience, client: &client, clock: cfg.Clock}, nil
}

func validURL(raw string) bool {
	u, err := url.Parse(raw)
	return err == nil && u.Scheme == "https" && u.Hostname() != "" && u.User == nil && u.RawQuery == "" && u.Fragment == "" && !u.ForceQuery
}

// Verify returns claims only after signature and registered-claim validation.
// It requires exp and validates nbf when present, with no clock-skew allowance.
func (v *Verifier) Verify(ctx context.Context, raw string) (ports.TokenClaims, error) {
	if ctx.Err() != nil || len(raw) == 0 || len(raw) > maxTokenBytes {
		return nil, ErrInvalidToken
	}
	parser := jwt.NewParser(jwt.WithValidMethods([]string{"RS256"}), jwt.WithIssuer(v.issuer), jwt.WithAudience(v.audience), jwt.WithExpirationRequired(), jwt.WithTimeFunc(v.clock.Now), jwt.WithJSONNumber(), jwt.WithStrictDecoding())
	token, err := parser.Parse(raw, func(t *jwt.Token) (any, error) {
		kid, ok := t.Header["kid"].(string)
		if !ok || kid == "" || len(kid) > 256 || t.Method != jwt.SigningMethodRS256 {
			return nil, ErrInvalidToken
		}
		// Never follow token-supplied jku/x5u or embedded jwk key material.
		return v.key(ctx, kid)
	})
	if err != nil || !token.Valid || ctx.Err() != nil {
		return nil, ErrInvalidToken
	}
	payload, err := base64.RawURLEncoding.DecodeString(strings.Split(raw, ".")[1])
	if err != nil {
		return nil, ErrInvalidToken
	}
	var claims ports.TokenClaims
	if json.Unmarshal(payload, &claims) != nil {
		return nil, ErrInvalidToken
	}
	return claims, nil
}

func (v *Verifier) key(ctx context.Context, kid string) (*rsa.PublicKey, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	now := v.clock.Now()
	if now.Before(v.expires) && v.keys[kid] != nil {
		return v.keys[kid], nil
	}
	if now.Before(v.nextRefresh) {
		return nil, ErrInvalidToken
	}
	v.nextRefresh = now.Add(refreshInterval)
	keys, err := v.discover(ctx)
	if err != nil {
		return nil, ErrInvalidToken
	}
	v.keys = keys
	v.expires = v.clock.Now().Add(keyLifetime)
	if keys[kid] == nil {
		return nil, ErrInvalidToken
	}
	return keys[kid], nil
}

func (v *Verifier) discover(ctx context.Context) (map[string]*rsa.PublicKey, error) {
	var metadata struct {
		Issuer  string `json:"issuer"`
		JWKSURI string `json:"jwks_uri"`
	}
	if err := v.fetch(ctx, strings.TrimRight(v.issuer, "/")+"/.well-known/openid-configuration", &metadata); err != nil {
		return nil, err
	}
	if metadata.Issuer != v.issuer || !validURL(metadata.JWKSURI) {
		return nil, ErrInvalidToken
	}
	var set struct {
		Keys []struct {
			Kty string   `json:"kty"`
			Kid string   `json:"kid"`
			Use string   `json:"use"`
			Alg string   `json:"alg"`
			Ops []string `json:"key_ops"`
			N   string   `json:"n"`
			E   string   `json:"e"`
		} `json:"keys"`
	}
	if err := v.fetch(ctx, metadata.JWKSURI, &set); err != nil {
		return nil, err
	}
	keys := make(map[string]*rsa.PublicKey)
	for _, k := range set.Keys {
		if k.Kty != "RSA" || k.Kid == "" || len(k.Kid) > 256 || (k.Use != "" && k.Use != "sig") || (k.Alg != "" && k.Alg != "RS256") {
			continue
		}
		if k.Ops != nil && (len(k.Ops) != 1 || k.Ops[0] != "verify") {
			continue
		}
		n, errN := base64.RawURLEncoding.Strict().DecodeString(k.N)
		e, errE := base64.RawURLEncoding.Strict().DecodeString(k.E)
		modulus := new(big.Int).SetBytes(n)
		exponent := new(big.Int).SetBytes(e)
		if errN != nil || errE != nil || modulus.BitLen() < 2048 || modulus.BitLen() > 8192 || !exponent.IsInt64() || exponent.Int64() < 3 || exponent.Int64() > 2147483647 || exponent.Bit(0) == 0 {
			return nil, ErrInvalidToken
		}
		if keys[k.Kid] != nil {
			return nil, ErrInvalidToken
		}
		keys[k.Kid] = &rsa.PublicKey{N: modulus, E: int(exponent.Int64())}
	}
	if len(keys) == 0 {
		return nil, ErrInvalidToken
	}
	return keys, nil
}

func (v *Verifier) fetch(ctx context.Context, address string, target any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, address, nil)
	if err != nil {
		return ErrInvalidToken
	}
	resp, err := v.client.Do(req)
	if err != nil {
		return ErrInvalidToken
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return ErrInvalidToken
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxDocumentBytes+1))
	if err != nil || len(body) > maxDocumentBytes || json.Unmarshal(body, target) != nil {
		return ErrInvalidToken
	}
	return nil
}
