package materialization

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/bdobrica/ThinkPixelWS/internal/domain"
	"github.com/bdobrica/ThinkPixelWS/internal/ports"
	"github.com/bdobrica/ThinkPixelWS/internal/security"
	"github.com/google/uuid"
)

type requestClockFunc func() time.Time

func (f requestClockFunc) Now() time.Time { return f() }

func TestMaterializationAuthorityLoss(t *testing.T) {
	now := time.Date(2026, 9, 27, 12, 0, 0, 0, time.UTC)
	id := func() uuid.UUID { return uuid.Must(uuid.NewV7()) }
	for _, mode := range []domain.MaterializationMode{domain.MaterializationReadOnly, domain.MaterializationReadWrite} {
		for _, loss := range []string{"expired", "revoked", "run cancelled", "unavailable"} {
			t.Run(string(mode)+"/"+loss, func(t *testing.T) {
				request := Request{Materialization: domain.NewMaterialization{
					TenantID: id(), ID: id(), WorkspaceID: id(), BaseGeneration: 1,
					Mode: mode, Provider: "kubernetes",
					Target: domain.MaterializationTarget{ID: "local", Region: "local", StorageClass: "local-path"},
				}, ComponentAccess: []ports.ExecutionComponentAccess{{ComponentID: id(), Mode: mode}}, ExecutionGrant: "private-grant"}
				authority := ports.ExecutionAuthority{Issuer: "ag", Audience: ports.ExecutionAuthorityAudience,
					GrantID: "grant", TenantID: request.Materialization.TenantID, Principal: "alice", RunID: id(),
					WorkspaceID: request.Materialization.WorkspaceID, ComponentAccess: request.ComponentAccess,
					IssuedAt: now.Add(-time.Minute), NotBefore: now.Add(-time.Minute), ExpiresAt: now.Add(time.Minute)}
				current := now
				var failure error
				calls := 0
				verifier := RequestScopeVerifier{
					Clock: requestClockFunc(func() time.Time { return current }),
					Verifier: requestAuthorityVerifier(func(_ context.Context, got ports.ExecutionAuthorityRequest) (ports.ExecutionAuthority, error) {
						calls++
						if got.TenantID != request.Materialization.TenantID || got.Grant != request.ExecutionGrant {
							t.Fatal("changed authenticated request")
						}
						// Even a faulty adapter returning claims with an error must fail closed.
						return authority, failure
					}),
				}
				if _, err := verifier.VerifiedMaterialization(t.Context(), request); err != nil {
					t.Fatal(err)
				}
				if loss == "expired" {
					current = authority.ExpiresAt // Equality is already expired; no grace period.
				} else {
					failure = errors.New(loss + ": " + request.ExecutionGrant)
				}
				got, err := verifier.Verify(t.Context(), request)
				if err != security.ErrExecutionAuthority || !reflect.DeepEqual(got, ports.ExecutionAuthority{}) {
					t.Fatal("retry exposed authority or verifier details after authority loss")
				}
				m, err := verifier.VerifiedMaterialization(t.Context(), request)
				if err != security.ErrExecutionAuthority || m != (domain.Materialization{}) || calls != 3 {
					t.Fatal("construction reused prior success after authority loss")
				}
				// Recovery requires fresh verifier success and a currently valid grant.
				failure = nil
				request.ExecutionGrant = "replacement-private-grant"
				authority.GrantID = "replacement-grant"
				authority.ExpiresAt = current.Add(time.Minute)
				if _, err := verifier.VerifiedMaterialization(t.Context(), request); err != nil || calls != 4 {
					t.Fatalf("fresh authority rejected: %v", err)
				}
			})
		}
	}
}

func TestMaterializationAuthorityExpiresBeforeReturn(t *testing.T) {
	now := time.Date(2026, 9, 27, 12, 0, 0, 0, time.UTC)
	id := func() uuid.UUID { return uuid.Must(uuid.NewV7()) }
	request := Request{Materialization: domain.NewMaterialization{
		TenantID: id(), ID: id(), WorkspaceID: id(), BaseGeneration: 1,
		Mode: domain.MaterializationReadWrite, Provider: "kubernetes",
		Target: domain.MaterializationTarget{ID: "local", Region: "local", StorageClass: "local-path"},
	}, ComponentAccess: []ports.ExecutionComponentAccess{{ComponentID: id(), Mode: domain.MaterializationReadWrite}}, ExecutionGrant: "private-grant"}
	authority := ports.ExecutionAuthority{Issuer: "ag", Audience: ports.ExecutionAuthorityAudience,
		GrantID: "grant", TenantID: request.Materialization.TenantID, Principal: "alice", RunID: id(),
		WorkspaceID: request.Materialization.WorkspaceID, ComponentAccess: request.ComponentAccess,
		IssuedAt: now.Add(-time.Minute), NotBefore: now.Add(-time.Minute), ExpiresAt: now.Add(time.Minute)}
	// Advance time at each boundary, including after verification has succeeded.
	for i, boundary := range []string{"AG verification", "scope validation", "request validation", "metadata construction"} {
		t.Run(boundary, func(t *testing.T) {
			reads := 0
			verifier := RequestScopeVerifier{
				Clock: requestClockFunc(func() time.Time {
					reads++
					if reads >= i+1 {
						return authority.ExpiresAt
					}
					return now
				}),
				Verifier: requestAuthorityVerifier(func(context.Context, ports.ExecutionAuthorityRequest) (ports.ExecutionAuthority, error) {
					return authority, nil
				}),
			}
			m, err := verifier.VerifiedMaterialization(t.Context(), request)
			if err != security.ErrExecutionAuthority || m != (domain.Materialization{}) {
				t.Fatal("expired authority produced metadata")
			}
		})
	}
}
