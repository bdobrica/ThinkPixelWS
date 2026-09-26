package security

import (
	"context"
	"errors"
	"testing"

	"github.com/bdobrica/ThinkPixelWS/internal/ports"
	"github.com/google/uuid"
)

type authorizerFunc func(context.Context, ports.WorkspaceAuthorizationRequest) (ports.WorkspaceAuthorizationDecision, error)

func (f authorizerFunc) Authorize(ctx context.Context, request ports.WorkspaceAuthorizationRequest) (ports.WorkspaceAuthorizationDecision, error) {
	return f(ctx, request)
}

func TestAuthorizeWorkspaceDecision(t *testing.T) {
	identity := Identity{TenantID: uuid.MustParse("01900000-0000-7000-8000-000000000001"), Principal: "alice"}
	workspaceID := uuid.MustParse("01900000-0000-7000-8000-000000000002")
	for _, tc := range []struct {
		name   string
		allow  bool
		err    error
		cancel bool
	}{
		{name: "explicit allow", allow: true},
		{name: "default deny"},
		{name: "policy failure", err: errors.New("sensitive policy detail")},
		{name: "allow with error", allow: true, err: errors.New("policy unavailable")},
		{name: "cancel during evaluation", allow: true, cancel: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			calls := 0
			authorizer := authorizerFunc(func(gotCtx context.Context, request ports.WorkspaceAuthorizationRequest) (ports.WorkspaceAuthorizationDecision, error) {
				calls++
				want := ports.WorkspaceAuthorizationRequest{TenantID: identity.TenantID, Principal: identity.Principal, Action: ports.WorkspaceView, WorkspaceID: workspaceID}
				if gotCtx != ctx || request != want {
					t.Fatalf("unexpected authorization scope: %+v", request)
				}
				if tc.cancel {
					cancel()
				}
				return ports.WorkspaceAuthorizationDecision{Allow: tc.allow}, tc.err
			})
			err := AuthorizeWorkspace(ctx, authorizer, identity, ports.WorkspaceView, workspaceID)
			if tc.allow && tc.err == nil && !tc.cancel {
				if err != nil {
					t.Fatal(err)
				}
			} else if err != ErrForbidden {
				t.Fatalf("want sanitized denial, got %v", err)
			}
			if calls != 1 {
				t.Fatalf("policy calls = %d", calls)
			}
		})
	}
}

func TestAuthorizeWorkspaceScope(t *testing.T) {
	tenant := uuid.MustParse("01900000-0000-7000-8000-000000000001")
	workspace := uuid.MustParse("01900000-0000-7000-8000-000000000002")
	for _, tc := range []struct {
		name     string
		identity Identity
		action   ports.WorkspaceAction
		target   uuid.UUID
		valid    bool
	}{
		{"create in tenant", Identity{tenant, "alice"}, ports.WorkspaceCreate, uuid.Nil, true},
		{"list in tenant", Identity{tenant, "alice"}, ports.WorkspaceList, uuid.Nil, true},
		{"materialization admin permission", Identity{tenant, "alice"}, ports.WorkspaceRequestMaterialization, workspace, true},
		{"missing tenant", Identity{uuid.Nil, "alice"}, ports.WorkspaceView, workspace, false},
		{"invalid tenant", Identity{uuid.New(), "alice"}, ports.WorkspaceView, workspace, false},
		{"missing principal", Identity{tenant, ""}, ports.WorkspaceView, workspace, false},
		{"invalid principal", Identity{tenant, " alice"}, ports.WorkspaceView, workspace, false},
		{"missing target", Identity{tenant, "alice"}, ports.WorkspaceView, uuid.Nil, false},
		{"invalid target", Identity{tenant, "alice"}, ports.WorkspaceDelete, uuid.New(), false},
		{"unexpected create target", Identity{tenant, "alice"}, ports.WorkspaceCreate, workspace, false},
		{"unknown action", Identity{tenant, "alice"}, "run.execute", workspace, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			called := false
			authorizer := authorizerFunc(func(_ context.Context, request ports.WorkspaceAuthorizationRequest) (ports.WorkspaceAuthorizationDecision, error) {
				called = true
				if request.TenantID != tc.identity.TenantID || request.Principal != tc.identity.Principal || request.WorkspaceID != tc.target || request.Action != tc.action {
					t.Fatal("authorization scope changed")
				}
				return ports.WorkspaceAuthorizationDecision{Allow: true}, nil
			})
			err := AuthorizeWorkspace(context.Background(), authorizer, tc.identity, tc.action, tc.target)
			if called != tc.valid || (err == nil) != tc.valid {
				t.Fatalf("called=%v, err=%v", called, err)
			}
		})
	}
	identity := Identity{tenant, "alice"}
	if err := AuthorizeWorkspace(context.Background(), nil, identity, ports.WorkspaceView, workspace); err != ErrForbidden {
		t.Fatal("nil authorizer must deny")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	authorizer := authorizerFunc(func(context.Context, ports.WorkspaceAuthorizationRequest) (ports.WorkspaceAuthorizationDecision, error) {
		t.Fatal("canceled request reached policy")
		return ports.WorkspaceAuthorizationDecision{}, nil
	})
	if err := AuthorizeWorkspace(ctx, authorizer, identity, ports.WorkspaceView, workspace); err != ErrForbidden {
		t.Fatal("cancellation must deny")
	}
}
