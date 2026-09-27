package security

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/bdobrica/ThinkPixelWS/internal/ports"
	"github.com/google/uuid"
)

type executionVerifierFunc func(context.Context, ports.ExecutionAuthorityRequest) (ports.ExecutionAuthority, error)

func (f executionVerifierFunc) VerifyExecutionAuthority(ctx context.Context, r ports.ExecutionAuthorityRequest) (ports.ExecutionAuthority, error) {
	return f(ctx, r)
}

type authorityClock struct{ now time.Time }

func (c *authorityClock) Now() time.Time { return c.now }

func TestExecutionAuthorityBoundary(t *testing.T) {
	now := time.Date(2026, 9, 27, 12, 0, 0, 0, time.UTC)
	id := uuid.MustParse("01900000-0000-7000-8000-000000000001")
	request := ports.ExecutionAuthorityRequest{TenantID: id, Grant: "opaque-sensitive-grant"}
	valid := ports.ExecutionAuthority{
		Issuer: "trusted-ag", Audience: ports.ExecutionAuthorityAudience, GrantID: "grant-1",
		TenantID: id, Principal: "alice", RunID: id, WorkspaceID: id,
		IssuedAt: now.Add(-time.Minute), NotBefore: now.Add(-time.Minute), ExpiresAt: now.Add(time.Minute),
	}
	for _, name := range []string{"valid", "missing verifier", "missing clock", "empty grant", "invalid tenant", "zero authority", "error with authority", "cancelled", "cancel during verification", "wrong tenant", "wrong audience", "expired during verification", "not yet valid", "future issuance", "missing issuer"} {
		t.Run(name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			c := &authorityClock{now}
			r := request
			a := valid
			calls := 0
			var verifier ports.ExecutionAuthorityVerifier = executionVerifierFunc(func(got context.Context, gotRequest ports.ExecutionAuthorityRequest) (ports.ExecutionAuthority, error) {
				calls++
				if got != ctx || gotRequest != request {
					t.Fatal("verification changed context or authenticated request")
				}
				switch name {
				case "zero authority":
					a = ports.ExecutionAuthority{}
				case "error with authority":
					return a, errors.New(request.Grant)
				case "cancel during verification":
					cancel()
				case "wrong tenant":
					a.TenantID = uuid.New()
				case "wrong audience":
					a.Audience = "thinkpixelar"
				case "expired during verification":
					c.now = a.ExpiresAt
				case "not yet valid":
					a.NotBefore = now.Add(time.Second)
				case "future issuance":
					a.IssuedAt = now.Add(time.Second)
				case "missing issuer":
					a.Issuer = ""
				}
				return a, nil
			})
			switch name {
			case "missing verifier":
				verifier = nil
			case "empty grant":
				r.Grant = " "
			case "invalid tenant":
				r.TenantID = uuid.Nil
			case "cancelled":
				cancel()
			}
			var got ports.ExecutionAuthority
			var err error
			if name == "missing clock" {
				got, err = VerifyExecutionAuthority(ctx, verifier, nil, r)
			} else {
				got, err = VerifyExecutionAuthority(ctx, verifier, c, r)
			}
			if name == "valid" {
				if err != nil || !reflect.DeepEqual(got, valid) {
					t.Fatalf("authority lost: %v", err)
				}
				_, err = VerifyExecutionAuthority(ctx, verifier, c, r)
				if err != nil || calls != 2 {
					t.Fatal("verification must be fresh on every call")
				}
			} else if err != ErrExecutionAuthority || !reflect.DeepEqual(got, ports.ExecutionAuthority{}) {
				t.Fatal("failure exposed authority or unsanitized error")
			}
		})
	}
}
