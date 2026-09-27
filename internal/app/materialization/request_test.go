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

type requestClock struct{ now time.Time }

func (c requestClock) Now() time.Time { return c.now }

type requestAuthorityVerifier func(context.Context, ports.ExecutionAuthorityRequest) (ports.ExecutionAuthority, error)

func (v requestAuthorityVerifier) VerifyExecutionAuthority(ctx context.Context, r ports.ExecutionAuthorityRequest) (ports.ExecutionAuthority, error) {
	return v(ctx, r)
}

func TestMaterializationRequestScope(t *testing.T) {
	tenant := uuid.Must(uuid.NewV7())
	workspace := uuid.Must(uuid.NewV7())
	one, two, other := uuid.Must(uuid.NewV7()), uuid.Must(uuid.NewV7()), uuid.Must(uuid.NewV7())
	now := time.Date(2026, 9, 27, 12, 0, 0, 0, time.UTC)
	access := func(id uuid.UUID) ports.ExecutionComponentAccess {
		return ports.ExecutionComponentAccess{ComponentID: id, Mode: domain.MaterializationReadOnly}
	}
	for _, tc := range []struct {
		name  string
		allow bool
	}{
		{"exact", true}, {"subset", true}, {"reordered", true},
		{"expanded", false}, {"substituted", false}, {"empty", false},
		{"duplicate ID with different mode", false}, {"wrong workspace", false},
		{"wrong generation", false}, {"wrong tenant", false}, {"missing grant", false},
		{"unavailable", false}, {"unconfigured", false},
		{"writable with read-only grant", false},
		{"writable with writable grant", true},
		{"writable with read-only component request", true},
		{"writable with mixed grant", false},
		{"writable subset with unused read-only grant component", true},
		{"read-only with writable grant", true},
		{"missing materialization mode", false}, {"unknown materialization mode", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			generation := uint64(3)
			request := Request{
				Materialization: domain.NewMaterialization{
					TenantID: tenant, ID: uuid.Must(uuid.NewV7()), WorkspaceID: workspace,
					BaseGeneration: generation, Mode: domain.MaterializationReadOnly, Provider: "kubernetes",
					Target: domain.MaterializationTarget{ID: "homelab", Region: "local", StorageClass: "local-path"},
				},
				ComponentAccess: []ports.ExecutionComponentAccess{access(one), access(two)}, ExecutionGrant: "private-grant",
			}
			authority := ports.ExecutionAuthority{
				Issuer: "trusted-ag", Audience: ports.ExecutionAuthorityAudience, GrantID: "grant-1", TenantID: tenant,
				Principal: "alice", RunID: uuid.Must(uuid.NewV7()), WorkspaceID: workspace, Generation: &generation,
				ComponentAccess: []ports.ExecutionComponentAccess{access(one), access(two)},
				IssuedAt:        now.Add(-time.Minute), NotBefore: now.Add(-time.Minute), ExpiresAt: now.Add(time.Minute),
			}
			switch tc.name {
			case "subset":
				request.ComponentAccess = request.ComponentAccess[:1]
			case "reordered":
				request.ComponentAccess[0], request.ComponentAccess[1] = request.ComponentAccess[1], request.ComponentAccess[0]
			case "expanded":
				request.ComponentAccess = append(request.ComponentAccess, access(other))
			case "substituted":
				request.ComponentAccess[1] = access(other)
			case "empty":
				request.ComponentAccess = nil
			case "duplicate ID with different mode":
				request.ComponentAccess[1] = ports.ExecutionComponentAccess{ComponentID: one, Mode: domain.MaterializationReadWrite}
			case "wrong workspace":
				request.Materialization.WorkspaceID = other
			case "wrong generation":
				request.Materialization.BaseGeneration++
			case "wrong tenant":
				authority.TenantID = other
			case "missing grant":
				request.ExecutionGrant = ""
			case "writable with read-only grant":
				// The overall mode must not bypass read-only component authority.
				request.Materialization.Mode = domain.MaterializationReadWrite
			case "writable with writable grant", "writable with read-only component request", "read-only with writable grant":
				for i := range authority.ComponentAccess {
					authority.ComponentAccess[i].Mode = domain.MaterializationReadWrite
				}
				if tc.name != "read-only with writable grant" {
					request.Materialization.Mode = domain.MaterializationReadWrite
				}
				if tc.name == "writable with writable grant" {
					for i := range request.ComponentAccess {
						request.ComponentAccess[i].Mode = domain.MaterializationReadWrite
					}
				}
			case "writable with mixed grant", "writable subset with unused read-only grant component":
				request.Materialization.Mode = domain.MaterializationReadWrite
				request.ComponentAccess[0].Mode = domain.MaterializationReadWrite
				authority.ComponentAccess[0].Mode = domain.MaterializationReadWrite
				if tc.name == "writable subset with unused read-only grant component" {
					request.ComponentAccess = request.ComponentAccess[:1]
				}
			case "missing materialization mode":
				request.Materialization.Mode = ""
			case "unknown materialization mode":
				request.Materialization.Mode = "write"
			}
			before := request
			if request.ComponentAccess != nil {
				before.ComponentAccess = append([]ports.ExecutionComponentAccess{}, request.ComponentAccess...)
			}
			calls := 0
			verifier := RequestScopeVerifier{Clock: requestClock{now}, Verifier: requestAuthorityVerifier(func(_ context.Context, received ports.ExecutionAuthorityRequest) (ports.ExecutionAuthority, error) {
				calls++
				if received.TenantID != tenant || received.Grant != request.ExecutionGrant {
					t.Fatal("verification did not use request tenant/grant")
				}
				if tc.name == "unavailable" {
					return authority, errors.New("private verifier details")
				}
				return authority, nil
			})}
			if tc.name == "unconfigured" {
				verifier.Verifier = nil
			}
			result, err := verifier.Verify(context.Background(), request)
			if tc.allow {
				if err != nil || !reflect.DeepEqual(result, authority) || calls != 1 {
					t.Fatalf("valid request denied: %v, calls %d", err, calls)
				}
				// Retrying the same request must consult current authority, not prior success.
				if request.Materialization.Mode == domain.MaterializationReadWrite {
					// A grant downgrade must also deny a retry whose component
					// request still says read-only but overall mode is writable.
					for i := range authority.ComponentAccess {
						authority.ComponentAccess[i].Mode = domain.MaterializationReadOnly
					}
				} else {
					authority.ComponentAccess = nil
				}
				result, err = verifier.Verify(context.Background(), request)
				if calls != 2 {
					t.Fatal("retry reused old authority")
				}
			}
			if !errors.Is(err, security.ErrExecutionAuthority) || !reflect.DeepEqual(result, ports.ExecutionAuthority{}) {
				t.Fatalf("denial returned authority or wrong error: %v", err)
			}
			if err.Error() != security.ErrExecutionAuthority.Error() {
				t.Fatal("denial leaked verifier details")
			}
			if !reflect.DeepEqual(request, before) {
				t.Fatal("verification changed requested scope")
			}
		})
	}
}

func TestVerifiedMaterializationReferences(t *testing.T) {
	now := time.Now().UTC()
	id := func() uuid.UUID { return uuid.Must(uuid.NewV7()) }
	request := Request{Materialization: domain.NewMaterialization{
		TenantID: id(), ID: id(), WorkspaceID: id(), BaseGeneration: 1,
		Mode: domain.MaterializationReadOnly, Provider: "kubernetes",
		Target: domain.MaterializationTarget{ID: "local", Region: "local", StorageClass: "local-path"},
	}, ComponentAccess: []ports.ExecutionComponentAccess{{ComponentID: id(), Mode: domain.MaterializationReadOnly}}, ExecutionGrant: "private-grant"}
	authority := ports.ExecutionAuthority{Issuer: "ag", Audience: ports.ExecutionAuthorityAudience,
		GrantID: "grant", TenantID: request.Materialization.TenantID, Principal: "alice", RunID: id(), ExecutionID: id(),
		WorkspaceID: request.Materialization.WorkspaceID, ComponentAccess: request.ComponentAccess,
		IssuedAt: now.Add(-time.Minute), NotBefore: now.Add(-time.Minute), ExpiresAt: now.Add(time.Minute)}
	calls := 0
	verifier := RequestScopeVerifier{Clock: requestClock{now}, Verifier: requestAuthorityVerifier(func(context.Context, ports.ExecutionAuthorityRequest) (ports.ExecutionAuthority, error) {
		calls++
		return authority, nil
	})}
	for _, execution := range []uuid.UUID{authority.ExecutionID, uuid.Nil} {
		authority.ExecutionID = execution
		m, err := verifier.VerifiedMaterialization(t.Context(), request)
		if err != nil || m.RunID != authority.RunID || m.ExecutionID != execution || m.State != domain.MaterializationRequested {
			t.Fatalf("verified record: %#v %v", m, err)
		}
		bound := request
		bound.Materialization.RunID, bound.Materialization.ExecutionID = m.RunID, m.ExecutionID
		if _, err := verifier.VerifiedMaterialization(t.Context(), bound); err != nil {
			t.Fatal(err)
		}
		for _, change := range []func(*Request){
			func(r *Request) { r.Materialization.RunID = id() },
			func(r *Request) { r.Materialization.ExecutionID = id() },
		} {
			wrong := bound
			change(&wrong)
			got, err := verifier.VerifiedMaterialization(t.Context(), wrong)
			if !errors.Is(err, security.ErrExecutionAuthority) || got != (domain.Materialization{}) {
				t.Fatalf("accepted conflicting reference: %#v %v", got, err)
			}
		}
	}
	authority.ExecutionID = uuid.New()
	if _, err := verifier.VerifiedMaterialization(t.Context(), request); !errors.Is(err, security.ErrExecutionAuthority) {
		t.Fatalf("invalid execution accepted: %v", err)
	}
	authority.ExecutionID = uuid.Nil
	authority.ExpiresAt = now
	if _, err := verifier.VerifiedMaterialization(t.Context(), request); !errors.Is(err, security.ErrExecutionAuthority) {
		t.Fatalf("expired retry accepted: %v", err)
	}
	if calls != 10 || request.Materialization.RunID != uuid.Nil || request.Materialization.ExecutionID != uuid.Nil {
		t.Fatal("verification cached or request mutated")
	}
}
