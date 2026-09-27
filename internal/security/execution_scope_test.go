package security

import (
	"context"
	"errors"
	"math"
	"reflect"
	"testing"
	"time"

	"github.com/bdobrica/ThinkPixelWS/internal/domain"
	"github.com/bdobrica/ThinkPixelWS/internal/ports"
	"github.com/google/uuid"
)

func TestExecutionScope(t *testing.T) {
	tenant := uuid.MustParse("01900000-0000-7000-8000-000000000001")
	workspace := uuid.MustParse("01900000-0000-7000-8000-000000000002")
	one := uuid.MustParse("01900000-0000-7000-8000-000000000003")
	two := uuid.MustParse("01900000-0000-7000-8000-000000000004")
	other := uuid.MustParse("01900000-0000-7000-8000-000000000005")
	now := time.Date(2026, 9, 27, 12, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		name  string
		allow bool
	}{
		{"exact scope", true}, {"proper subset", true}, {"reordered", true}, {"unpinned generation", true},
		{"read-only under writable grant", true}, {"grant downgraded on next verification", true},
		{"writable under read-only grant", false}, {"omitted request mode", false}, {"unknown request mode", false},
		{"omitted grant mode", false}, {"unknown grant mode", false}, {"invalid unused grant mode", false},
		{"wrong workspace", false}, {"wrong generation", false}, {"expanded components", false},
		{"empty request", false}, {"empty grant", false}, {"duplicate request", false}, {"duplicate grant", false},
		{"invalid workspace", false}, {"invalid request component", false}, {"invalid grant component", false},
		{"zero generation", false}, {"overflow generation", false}, {"zero grant generation", false}, {"overflow grant generation", false},
		{"wrong tenant", false}, {"expired", false}, {"unavailable", false}, {"cancelled", false}, {"missing verifier", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			generation := uint64(3)
			request := ports.ExecutionAuthorityRequest{TenantID: tenant, Grant: "sensitive-grant"}
			scope := ExecutionScope{WorkspaceID: workspace, Generation: 3, ComponentAccess: []ports.ExecutionComponentAccess{{ComponentID: one, Mode: domain.MaterializationReadOnly}, {ComponentID: two, Mode: domain.MaterializationReadWrite}}}
			a := ports.ExecutionAuthority{
				Issuer: "trusted-ag", Audience: ports.ExecutionAuthorityAudience, GrantID: "grant-1",
				TenantID: tenant, Principal: "alice", RunID: other, WorkspaceID: workspace, Generation: &generation,
				ComponentAccess: []ports.ExecutionComponentAccess{{ComponentID: one, Mode: domain.MaterializationReadOnly}, {ComponentID: two, Mode: domain.MaterializationReadWrite}},
				IssuedAt:        now.Add(-time.Minute), NotBefore: now.Add(-time.Minute), ExpiresAt: now.Add(time.Minute),
			}
			switch tc.name {
			case "read-only under writable grant":
				scope.ComponentAccess[1].Mode = domain.MaterializationReadOnly
			case "writable under read-only grant":
				scope.ComponentAccess[0].Mode = domain.MaterializationReadWrite
			case "omitted request mode":
				scope.ComponentAccess[0].Mode = ""
			case "unknown request mode":
				scope.ComponentAccess[0].Mode = "write"
			case "omitted grant mode":
				a.ComponentAccess[0].Mode = ""
			case "unknown grant mode":
				a.ComponentAccess[0].Mode = "write"
			case "invalid unused grant mode":
				a.ComponentAccess = append(a.ComponentAccess, ports.ExecutionComponentAccess{ComponentID: other, Mode: "admin"})
			case "proper subset":
				scope.ComponentAccess = scope.ComponentAccess[1:]
			case "reordered":
				scope.ComponentAccess[0], scope.ComponentAccess[1] = scope.ComponentAccess[1], scope.ComponentAccess[0]
			case "unpinned generation":
				a.Generation = nil
				scope.Generation = 42
			case "wrong workspace":
				scope.WorkspaceID = other
			case "wrong generation":
				scope.Generation++
			case "expanded components":
				scope.ComponentAccess = append(scope.ComponentAccess, ports.ExecutionComponentAccess{ComponentID: other, Mode: domain.MaterializationReadOnly})
			case "empty request":
				scope.ComponentAccess = nil
			case "empty grant":
				a.ComponentAccess = nil
			case "duplicate request":
				scope.ComponentAccess = append(scope.ComponentAccess, scope.ComponentAccess[0])
			case "duplicate grant":
				a.ComponentAccess = append(a.ComponentAccess, ports.ExecutionComponentAccess{ComponentID: one, Mode: domain.MaterializationReadWrite})
			case "invalid workspace":
				scope.WorkspaceID = uuid.Nil
			case "invalid request component":
				scope.ComponentAccess[0].ComponentID = uuid.New()
			case "invalid grant component":
				a.ComponentAccess = append(a.ComponentAccess, ports.ExecutionComponentAccess{ComponentID: uuid.Nil, Mode: domain.MaterializationReadOnly})
			case "zero generation":
				scope.Generation = 0
			case "overflow generation":
				scope.Generation = math.MaxUint64
			case "zero grant generation":
				generation = 0
			case "overflow grant generation":
				generation = math.MaxUint64
			case "wrong tenant":
				a.TenantID = other
			case "expired":
				a.ExpiresAt = now
			case "cancelled":
				cancel()
			}
			calls := 0
			var verifier ports.ExecutionAuthorityVerifier = executionVerifierFunc(func(got context.Context, r ports.ExecutionAuthorityRequest) (ports.ExecutionAuthority, error) {
				calls++
				if got != ctx || r != request {
					t.Fatal("changed verification context or tenant/grant")
				}
				if tc.name == "unavailable" {
					return a, errors.New(request.Grant)
				}
				// A subsequent downgrade or revocation must not reuse prior authority.
				if calls > 1 && tc.name == "grant downgraded on next verification" {
					a.ComponentAccess[1].Mode = domain.MaterializationReadOnly
				} else if calls > 1 {
					return ports.ExecutionAuthority{}, errors.New("revoked")
				}
				return a, nil
			})
			if tc.name == "missing verifier" {
				verifier = nil
			}
			got, err := VerifyExecutionScope(ctx, verifier, &authorityClock{now}, request, scope)
			if !tc.allow {
				if err != ErrExecutionAuthority || !reflect.DeepEqual(got, ports.ExecutionAuthority{}) {
					t.Fatal("denial returned authority or unsanitized error")
				}
				return
			}
			if err != nil || !reflect.DeepEqual(got, a) {
				t.Fatalf("valid scope denied or authority altered: %v", err)
			}
			got, err = VerifyExecutionScope(ctx, verifier, &authorityClock{now}, request, scope)
			if calls != 2 || err != ErrExecutionAuthority || !reflect.DeepEqual(got, ports.ExecutionAuthority{}) {
				t.Fatal("reused stale authority")
			}
		})
	}
}
