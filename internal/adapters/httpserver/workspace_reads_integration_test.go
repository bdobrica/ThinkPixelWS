package httpserver

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	api "github.com/bdobrica/ThinkPixelWS/api/openapi"
	clockadapter "github.com/bdobrica/ThinkPixelWS/internal/adapters/clock"
	"github.com/bdobrica/ThinkPixelWS/internal/adapters/postgres"
	"github.com/bdobrica/ThinkPixelWS/internal/app/workspace"
	"github.com/bdobrica/ThinkPixelWS/internal/domain"
	"github.com/bdobrica/ThinkPixelWS/internal/ports"
	"github.com/bdobrica/ThinkPixelWS/internal/security"
	"github.com/google/uuid"
)

type readClock struct{ now time.Time }

func (c *readClock) Now() time.Time { return c.now }

type readPolicy struct {
	denyList bool
	hidden   uuid.UUID
}

func (p readPolicy) Authorize(_ context.Context, req ports.WorkspaceAuthorizationRequest) (ports.WorkspaceAuthorizationDecision, error) {
	return ports.WorkspaceAuthorizationDecision{Allow: !(p.denyList && req.Action == ports.WorkspaceList) && req.WorkspaceID != p.hidden}, nil
}

func TestReadWorkspacesPostgres(t *testing.T) {
	db := workspaceTestDatabase(t)
	cfg, token := developmentConfig(t)
	tenant := uuid.MustParse(cfg.Auth.TenantID)
	otherTenant, _ := uuid.NewV7()
	for _, id := range []uuid.UUID{tenant, otherTenant} {
		if _, err := db.ExecContext(t.Context(), `INSERT INTO thinkpixelws.tenants (tenant_id,lifecycle_state) VALUES ($1,'ACTIVE')`, id); err != nil {
			t.Fatal(err)
		}
	}
	clock := &readClock{now: time.Now().UTC()}
	codec, err := security.NewCursorCodec(bytes.Repeat([]byte{7}, 32), clock)
	if err != nil {
		t.Fatal(err)
	}
	creator := workspace.Creator{Store: postgres.WorkspaceCreator{DB: db}, Clock: clockadapter.System{}}
	reader := workspace.Reader{Store: postgres.WorkspaceReader{DB: db}, Cursors: codec, Clock: clock}
	handler, err := NewWorkspaceAPI(creator, reader)
	if err != nil {
		t.Fatal(err)
	}
	server, err := New(cfg, dependencies(handler))
	if err != nil {
		t.Fatal(err)
	}
	live := httptest.NewServer(server.public.Handler)
	defer live.Close()
	get := func(path string, status int) []byte {
		t.Helper()
		req, _ := http.NewRequest("GET", live.URL+path, nil)
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("X-Tenant-ID", otherTenant.String())
		res, err := live.Client().Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		body, _ := io.ReadAll(res.Body)
		if res.StatusCode != status {
			t.Fatalf("%s: %d %s", path, res.StatusCode, body)
		}
		return body
	}
	var empty api.WorkspacePage
	if err = json.Unmarshal(get("/v1/workspaces", 200), &empty); err != nil || len(empty.Items) != 0 {
		t.Fatalf("empty page: %+v %v", empty, err)
	}
	var ids []uuid.UUID
	for i := 0; i < 5; i++ {
		scope := tenant
		if i == 1 {
			scope = otherTenant
		}
		id, _ := uuid.NewV7()
		record, err := (domain.NewWorkspace{TenantID: scope, ID: id, Name: "demo", Owner: domain.Owner{Kind: domain.OwnerUser, ID: "alice"}}).Workspace(clock.now)
		if err != nil {
			t.Fatal(err)
		}
		if err = postgres.NewWorkspaceRepository(db).Create(t.Context(), scope, record); err != nil {
			t.Fatal(err)
		}
		if scope == tenant {
			ids = append(ids, id)
		} else {
			get("/v1/workspaces/"+id.String(), 404)
		}
	}
	var first api.WorkspacePage
	if err = json.Unmarshal(get("/v1/workspaces?limit=2", 200), &first); err != nil {
		t.Fatal(err)
	}
	if len(first.Items) != 2 || uuid.UUID(first.Items[0].ID) != ids[0] || uuid.UUID(first.Items[1].ID) != ids[1] || !first.NextCursor.Set {
		t.Fatalf("first page: %+v", first)
	}
	cursor := first.NextCursor.Value
	// Removing the boundary row must not skip the following row (no OFFSET).
	if _, err = db.ExecContext(t.Context(), `DELETE FROM thinkpixelws.workspaces WHERE tenant_id=$1 AND workspace_id=$2`, tenant, ids[1]); err != nil {
		t.Fatal(err)
	}
	var second api.WorkspacePage
	if err = json.Unmarshal(get("/v1/workspaces?limit=2&cursor="+url.QueryEscape(cursor), 200), &second); err != nil {
		t.Fatal(err)
	}
	if len(second.Items) != 2 || uuid.UUID(second.Items[0].ID) != ids[2] || uuid.UUID(second.Items[1].ID) != ids[3] || second.NextCursor.Set {
		t.Fatalf("second page: %+v", second)
	}
	var got api.Workspace
	if err = json.Unmarshal(get("/v1/workspaces/"+ids[2].String(), 200), &got); err != nil || uuid.UUID(got.ID) != ids[2] || got.HeadGeneration.Set {
		t.Fatalf("get: %+v %v", got, err)
	}

	// Read the persisted head rather than manufacturing or omitting it.
	generationID, _ := uuid.NewV7()
	if _, err = db.ExecContext(t.Context(), `INSERT INTO thinkpixelws.workspace_generations
 (tenant_id,workspace_id,generation,generation_id,state,manifest_digest,durability,created_by_principal)
 VALUES ($1,$2,1,$3,'COMPLETED',$4,'provider-local','alice')`, tenant, ids[2], generationID, "sha256:"+strings.Repeat("a", 64)); err != nil {
		t.Fatal(err)
	}
	if _, err = db.ExecContext(t.Context(), `UPDATE thinkpixelws.workspaces SET head_generation=1,lifecycle_state='READY',state_version=2 WHERE tenant_id=$1 AND workspace_id=$2`, tenant, ids[2]); err != nil {
		t.Fatal(err)
	}
	json.Unmarshal(get("/v1/workspaces/"+ids[2].String(), 200), &got)
	if !got.HeadGeneration.Set || got.HeadGeneration.Value != 1 || got.StateVersion != 2 {
		t.Fatalf("stored head/state missing: %+v", got)
	}
	missing, _ := uuid.NewV7()
	get("/v1/workspaces/"+missing.String(), 404)
	for _, path := range []string{"/v1/workspaces?limit=0", "/v1/workspaces?limit=501", "/v1/workspaces?cursor=", "/v1/workspaces?cursor=" + url.QueryEscape("x"+cursor), "/v1/workspaces/not-a-uuid"} {
		get(path, 400)
	}
	get("/v1/workspaces?limit=500", 200)
	// Authentication is still required for both read routes.
	for _, path := range []string{"/v1/workspaces", "/v1/workspaces/" + ids[0].String()} {
		res, err := live.Client().Get(live.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		res.Body.Close()
		if res.StatusCode != 401 {
			t.Fatalf("unauthenticated: %d", res.StatusCode)
		}
	}
	// Same key/codec but a different caller or tenant cannot reuse the cursor.
	for _, identity := range []security.Identity{{TenantID: tenant, Principal: "other"}, {TenantID: otherTenant, Principal: cfg.Auth.Principal}} {
		_, err := reader.List(t.Context(), identity.TenantID, identity.Principal, cursor, 2)
		if err == nil {
			t.Fatal("cursor scope escaped")
		}
	}
	// Each request reevaluates policy, including cursor continuation.
	policyCall := func(path string, p ports.WorkspaceAuthorizer, status int) []byte {
		t.Helper()
		req := httptest.NewRequest("GET", path, nil)
		ctx := context.WithValue(req.Context(), accessContextKey{}, requestAccess{identity: security.Identity{TenantID: tenant, Principal: cfg.Auth.Principal}, authorizer: p})
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req.WithContext(ctx))
		if rec.Code != status {
			t.Fatalf("policy response: %d %s", rec.Code, rec.Body.String())
		}
		return rec.Body.Bytes()
	}
	policyCall("/v1/workspaces", nil, 403)
	policyCall("/v1/workspaces?cursor="+url.QueryEscape(cursor), readPolicy{denyList: true}, 403)
	policyCall("/v1/workspaces/"+ids[0].String(), readPolicy{hidden: ids[0]}, 403)
	var filtered api.WorkspacePage
	json.Unmarshal(policyCall("/v1/workspaces?limit=1", readPolicy{hidden: ids[0]}, 200), &filtered)
	if len(filtered.Items) != 0 || !filtered.NextCursor.Set {
		t.Fatalf("filtered page must permit continuation: %+v", filtered)
	}
	clock.now = clock.now.Add(16 * time.Minute)
	get("/v1/workspaces?cursor="+url.QueryEscape(cursor), 410)
	// Reads do not create audit/outbox mutation records.
	for _, table := range []string{"audit_events", "outbox_messages", "idempotency_records"} {
		var n int
		if err = db.QueryRowContext(t.Context(), "SELECT count(*) FROM thinkpixelws."+table).Scan(&n); err != nil || n != 0 {
			t.Fatalf("read side effects: %s %d %v", table, n, err)
		}
	}
	t.Log("verified live HTTP reads, keyset pagination after deletion, tenant isolation, policy, and authenticated cursor boundaries")
}
