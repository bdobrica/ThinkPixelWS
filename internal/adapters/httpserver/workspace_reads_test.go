package httpserver

import (
	"bytes"
	"context"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"

	clockadapter "github.com/bdobrica/ThinkPixelWS/internal/adapters/clock"
	"github.com/bdobrica/ThinkPixelWS/internal/app/workspace"
	"github.com/bdobrica/ThinkPixelWS/internal/ports"
	"github.com/bdobrica/ThinkPixelWS/internal/security"
	"github.com/google/uuid"
)

type readStore struct {
	tenant       uuid.UUID
	limit, calls int
	err          error
}

func (s *readStore) GetWorkspace(_ context.Context, tenant, id uuid.UUID) (ports.WorkspaceRecord, error) {
	s.tenant = tenant
	s.calls++
	if s.err != nil {
		return ports.WorkspaceRecord{}, s.err
	}
	return ports.WorkspaceRecord{}, ports.ErrWorkspaceNotFound
}
func (s *readStore) ListWorkspaces(_ context.Context, tenant, after uuid.UUID, limit int) ([]ports.WorkspaceRecord, error) {
	s.tenant = tenant
	s.limit = limit
	s.calls++
	return nil, s.err
}
func TestWorkspaceReadHTTPBoundaries(t *testing.T) {
	cfg, token := developmentConfig(t)
	store := &readStore{}
	codec, _ := security.NewCursorCodec(bytes.Repeat([]byte{1}, 32), clockadapter.System{})
	handler, err := NewWorkspaceAPI(workspace.Creator{}, workspace.Reader{Store: store, Cursors: codec, Clock: clockadapter.System{}})
	if err != nil {
		t.Fatal(err)
	}
	server, err := New(cfg, dependencies(handler))
	if err != nil {
		t.Fatal(err)
	}
	call := func(path string, status int) {
		t.Helper()
		req := httptest.NewRequest("GET", path, nil)
		req.RemoteAddr = "127.0.0.1:1234"
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("X-Tenant-ID", uuid.NewString())
		rec := httptest.NewRecorder()
		server.public.Handler.ServeHTTP(rec, req)
		if rec.Code != status {
			t.Fatalf("%s: %d %s", path, rec.Code, rec.Body.String())
		}
		if strings.Contains(rec.Body.String(), "secret-database-error") {
			t.Fatal("database error leaked")
		}
	}
	call("/v1/workspaces", 200)
	if store.limit != 101 || store.tenant.String() != cfg.Auth.TenantID {
		t.Fatalf("default limit/tenant: %+v", store)
	}
	for _, path := range []string{"/v1/workspaces?limit=0", "/v1/workspaces?limit=501", "/v1/workspaces?cursor=invalid", "/v1/workspaces?cursor="} {
		call(path, 400)
	}
	if store.calls != 1 {
		t.Fatal("invalid request reached database")
	}
	id, _ := uuid.NewV7()
	call("/v1/workspaces/"+id.String(), 404)
	store.err = errors.New("secret-database-error")
	call("/v1/workspaces", 503)
	call("/v1/workspaces/"+id.String(), 503)
	// Missing policy must reject list before touching persistence.
	req := httptest.NewRequest("GET", "/v1/workspaces", nil)
	ctx := context.WithValue(req.Context(), accessContextKey{}, requestAccess{identity: security.Identity{TenantID: uuid.MustParse(cfg.Auth.TenantID), Principal: cfg.Auth.Principal}})
	before := store.calls
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req.WithContext(ctx))
	if rec.Code != 403 || store.calls != before {
		t.Fatal("list policy did not precede database access")
	}
}
