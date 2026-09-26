package security

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/bdobrica/ThinkPixelWS/internal/ports"
	"github.com/google/uuid"
)

func TestDevelopmentAuth(t *testing.T) {
	identity := Identity{TenantID: uuid.MustParse("0198cb17-3520-7000-8000-000000000001"), Principal: "local-developer"}
	token := strings.Repeat("test-token", 4)
	auth, err := NewDevelopmentAuth(identity, token)
	if err != nil {
		t.Fatal(err)
	}
	for _, credential := range []string{token, "", token + "x"} {
		got, err := auth.Authenticate(context.Background(), credential)
		if credential == token {
			if err != nil || got != identity {
				t.Fatalf("identity = %+v, error = %v", got, err)
			}
		} else if !errors.Is(err, ErrUnauthenticated) || got != (Identity{}) {
			t.Fatal("invalid credential accepted")
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := auth.Authenticate(ctx, token); !errors.Is(err, ErrUnauthenticated) {
		t.Fatal("cancelled authentication accepted")
	}
	workspace := uuid.MustParse("0198cb17-3520-7000-8000-000000000002")
	for _, tc := range []struct {
		action ports.WorkspaceAction
		target uuid.UUID
		allow  bool
	}{
		{ports.WorkspaceCreate, uuid.Nil, true}, {ports.WorkspaceList, uuid.Nil, true}, {ports.WorkspaceView, workspace, true},
		{ports.WorkspaceCreate, workspace, false}, {ports.WorkspaceView, uuid.Nil, false},
		{ports.WorkspaceDelete, workspace, false}, {ports.WorkspaceFork, workspace, false},
		{ports.WorkspaceArchive, workspace, false}, {ports.WorkspaceRestore, workspace, false},
		{ports.WorkspaceRequestMaterialization, workspace, false}, {ports.WorkspaceReadProfileMetadata, workspace, false},
		{"run.execute", workspace, false},
	} {
		err := AuthorizeWorkspace(context.Background(), auth, identity, tc.action, tc.target)
		if (err == nil) != tc.allow {
			t.Errorf("%s %s: %v", tc.action, tc.target, err)
		}
	}
	for _, caller := range []Identity{{TenantID: workspace, Principal: identity.Principal}, {TenantID: identity.TenantID, Principal: "other"}} {
		if err := AuthorizeWorkspace(context.Background(), auth, caller, ports.WorkspaceList, uuid.Nil); !errors.Is(err, ErrForbidden) {
			t.Fatal("foreign identity allowed")
		}
	}
	if err := AuthorizeWorkspace(ctx, auth, identity, ports.WorkspaceList, uuid.Nil); !errors.Is(err, ErrForbidden) {
		t.Fatal("cancelled authorization allowed")
	}
	for _, bad := range []string{"", "short", strings.Repeat("x", 257), strings.Repeat("x", 32) + "\n"} {
		if _, err := NewDevelopmentAuth(identity, bad); err == nil {
			t.Fatal("invalid token configured")
		}
	}
	if _, err := NewDevelopmentAuth(Identity{}, token); err == nil {
		t.Fatal("invalid identity configured")
	}
}
