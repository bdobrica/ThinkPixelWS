package httpserver

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	api "github.com/bdobrica/ThinkPixelWS/api/openapi"
	"github.com/bdobrica/ThinkPixelWS/internal/adapters/postgres"
	"github.com/bdobrica/ThinkPixelWS/internal/app/workspace"
	"github.com/bdobrica/ThinkPixelWS/internal/domain"
	"github.com/bdobrica/ThinkPixelWS/internal/ports"
	"github.com/bdobrica/ThinkPixelWS/internal/security"
	"github.com/google/uuid"
)

func TestReadWorkspaceMetadataPostgres(t *testing.T) {
	db := workspaceTestDatabase(t)
	cfg, token := developmentConfig(t)
	tenant := uuid.MustParse(cfg.Auth.TenantID)
	otherTenant := uuid.Must(uuid.NewV7())
	clock := &readClock{now: time.Now().UTC()}
	codec, err := security.NewCursorCodec(bytes.Repeat([]byte{9}, 32), clock)
	if err != nil {
		t.Fatal(err)
	}
	store := postgres.WorkspaceReader{DB: db}
	reader := workspace.Reader{Store: store, Metadata: store, Cursors: codec, Clock: clock}
	handler, err := NewWorkspaceAPI(workspace.Creator{}, reader)
	if err != nil {
		t.Fatal(err)
	}
	server, err := New(cfg, dependencies(handler))
	if err != nil {
		t.Fatal(err)
	}
	live := httptest.NewServer(server.public.Handler)
	defer live.Close()
	exec := func(query string, args ...any) {
		t.Helper()
		if _, err := db.ExecContext(t.Context(), query, args...); err != nil {
			t.Fatal(err)
		}
	}
	get := func(path string, status int) ([]byte, string) {
		t.Helper()
		req, _ := http.NewRequest("GET", live.URL+path, nil)
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("X-Tenant-ID", otherTenant.String())
		res, err := live.Client().Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		body, err := io.ReadAll(res.Body)
		if err != nil {
			t.Fatal(err)
		}
		if res.StatusCode != status {
			t.Fatalf("%s: %d %s", path, res.StatusCode, body)
		}
		return body, res.Header.Get("Next-Cursor")
	}
	for _, id := range []uuid.UUID{tenant, otherTenant} {
		exec(`INSERT INTO thinkpixelws.tenants(tenant_id,lifecycle_state) VALUES($1,'ACTIVE')`, id)
	}
	workspaces := []uuid.UUID{uuid.Must(uuid.NewV7()), uuid.Must(uuid.NewV7()), uuid.Must(uuid.NewV7())}
	for i, id := range workspaces {
		scope := tenant
		if i == 2 {
			scope = otherTenant
		}
		w, err := (domain.NewWorkspace{TenantID: scope, ID: id, Name: "demo", Owner: domain.Owner{Kind: domain.OwnerUser, ID: "alice"}}).Workspace(clock.now)
		if err != nil {
			t.Fatal(err)
		}
		if err = postgres.NewWorkspaceRepository(db).Create(t.Context(), scope, w); err != nil {
			t.Fatal(err)
		}
	}
	base := "/v1/workspaces/" + workspaces[0].String()
	otherBase := "/v1/workspaces/" + workspaces[1].String()
	foreignBase := "/v1/workspaces/" + workspaces[2].String()
	for _, kind := range []string{"components", "generations"} {
		body, next := get(base+"/"+kind, 200)
		if string(body) != "[]" || next != "" {
			t.Fatalf("empty %s: %s %q", kind, body, next)
		}
	}
	var components []uuid.UUID
	digest := "sha256:" + strings.Repeat("a", 64)
	for i := 0; i < 3; i++ {
		component := uuid.Must(uuid.NewV7())
		components = append(components, component)
		exec(`INSERT INTO thinkpixelws.workspace_components(tenant_id,workspace_id,component_id,name,kind,canonical_path) VALUES($1,$2,$3,$4,'directory',$5)`, tenant, workspaces[0], component, fmt.Sprintf("part-%d", i), fmt.Sprintf("/workspace/part-%d", i))
		var parent any
		if i > 0 {
			parent = i
		}
		exec(`INSERT INTO thinkpixelws.workspace_generations(tenant_id,workspace_id,generation_id,generation,parent_generation,state,manifest_digest,durability,created_by_principal) VALUES($1,$2,$3,$4,$5,'COMPLETED',$6,'portable','alice')`, tenant, workspaces[0], uuid.Must(uuid.NewV7()), i+1, parent, digest)
	}
	exec(`INSERT INTO thinkpixelws.source_bindings(tenant_id,workspace_id,component_id,provider,source_ref,mode,last_resolved_revision) VALUES($1,$2,$3,'git','https://example.com/repo','snapshot','revision-one')`, tenant, workspaces[0], components[0])
	exec(`INSERT INTO thinkpixelws.component_classification_metadata(tenant_id,workspace_id,generation,component_id,classification,taints) VALUES($1,$2,1,$3,'internal','{}'),($1,$2,3,$3,'confidential','{high-taint}')`, tenant, workspaces[0], components[0])
	exec(`UPDATE thinkpixelws.workspaces SET head_generation=3 WHERE tenant_id=$1 AND workspace_id=$2`, tenant, workspaces[0])
	body, _ := get(base+"/components/"+components[0].String(), 200)
	var component api.Component
	if err = json.Unmarshal(body, &component); err != nil {
		t.Fatal(err)
	}
	if component.ID != api.UUID(components[0]) || component.Path != "/workspace/part-0" || !component.Source.Set || component.Source.Value.Ref.String() != "https://example.com/repo" || component.Source.Value.Revision.Value != "revision-one" || component.Classification.Value != api.ClassificationConfidential || len(component.Taints) != 1 || component.Taints[0] != "high-taint" {
		t.Fatalf("component: %s", body)
	}
	body, _ = get(base+"/components/"+components[1].String(), 200)
	var bare map[string]any
	if err = json.Unmarshal(body, &bare); err != nil {
		t.Fatal(err)
	}
	if _, ok := bare["source"]; ok {
		t.Fatalf("invented source: %s", body)
	}
	if _, ok := bare["classification"]; ok {
		t.Fatalf("invented classification: %s", body)
	}
	body, _ = get(base+"/generations/3", 200)
	var generation api.Generation
	if err = json.Unmarshal(body, &generation); err != nil {
		t.Fatal(err)
	}
	if generation.Number != 3 || generation.ParentGeneration.Value != 2 || generation.WorkspaceId != api.UUID(workspaces[0]) || generation.State != "COMPLETED" || generation.ManifestDigest != digest || string(generation.Durability) != `"portable"` || generation.CreatedAt.IsZero() {
		t.Fatalf("generation: %s", body)
	}
	body, _ = get(base+"/generations/1", 200)
	var first map[string]any
	if err = json.Unmarshal(body, &first); err != nil {
		t.Fatal(err)
	}
	if _, ok := first["parentGeneration"]; ok {
		t.Fatalf("invented parent: %s", body)
	}
	for _, kind := range []string{"components", "generations"} {
		t.Run(kind, func(t *testing.T) {
			body, cursor := get(base+"/"+kind+"?limit=1", 200)
			var items []map[string]any
			if err := json.Unmarshal(body, &items); err != nil || len(items) != 1 || cursor == "" {
				t.Fatalf("first page: %s %s %v", body, cursor, err)
			}
			if kind == "components" && items[0]["id"] != components[0].String() {
				t.Fatalf("order: %s", body)
			}
			if kind == "generations" && items[0]["number"] != float64(1) {
				t.Fatalf("order: %s", body)
			}
			// Reconstruct the API with the same key to prove persisted read/cursor state.
			restarted, err := NewWorkspaceAPI(workspace.Creator{}, reader)
			if err != nil {
				t.Fatal(err)
			}
			req := httptest.NewRequest("GET", base+"/"+kind+"?limit=2&cursor="+url.QueryEscape(cursor), nil)
			req = req.WithContext(context.WithValue(req.Context(), accessContextKey{}, requestAccess{identity: security.Identity{TenantID: tenant, Principal: cfg.Auth.Principal}, authorizer: readPolicy{}}))
			rec := httptest.NewRecorder()
			restarted.ServeHTTP(rec, req)
			if rec.Code != 200 || rec.Header().Get("Next-Cursor") != "" {
				t.Fatalf("restart continuation: %d %s", rec.Code, rec.Body.String())
			}
			if err = json.Unmarshal(rec.Body.Bytes(), &items); err != nil || len(items) != 2 {
				t.Fatalf("continuation: %s %v", rec.Body.String(), err)
			}
			if kind == "components" && (items[0]["id"] != components[1].String() || items[1]["id"] != components[2].String()) {
				t.Fatal("component continuation order")
			}
			if kind == "generations" && (items[0]["number"] != float64(2) || items[1]["number"] != float64(3)) {
				t.Fatal("generation continuation order")
			}
			get(otherBase+"/"+kind+"?cursor="+url.QueryEscape(cursor), 400)
			otherKind := "components"
			if kind == otherKind {
				otherKind = "generations"
			}
			get(base+"/"+otherKind+"?cursor="+url.QueryEscape(cursor), 400)
			get(base+"/"+kind+"?cursor="+url.QueryEscape(cursor+"x"), 400)
			for _, query := range []string{"?cursor=", "?cursor=bad", "?limit=0", "?limit=501"} {
				get(base+"/"+kind+query, 400)
			}
			get(base+"/"+kind+"?limit=500", 200)
			for _, identity := range []security.Identity{{TenantID: tenant, Principal: "different"}, {TenantID: otherTenant, Principal: cfg.Auth.Principal}} {
				var scopeErr error
				if kind == "components" {
					_, _, scopeErr = reader.ListComponents(t.Context(), identity.TenantID, workspaces[0], identity.Principal, cursor, 1)
				} else {
					_, _, scopeErr = reader.ListGenerations(t.Context(), identity.TenantID, workspaces[0], identity.Principal, cursor, 1)
				}
				if scopeErr == nil {
					t.Fatal("cursor accepted in another identity scope")
				}
			}
			clock.now = clock.now.Add(16 * time.Minute)
			get(base+"/"+kind+"?cursor="+url.QueryEscape(cursor), 410)
		})
	}
	for _, suffix := range []string{"/components", "/generations", "/components/" + components[0].String(), "/generations/1"} {
		get(foreignBase+suffix, 404)
		get("/v1/workspaces/"+uuid.Must(uuid.NewV7()).String()+suffix, 404)
		req := httptest.NewRequest("GET", base+suffix, nil)
		rec := httptest.NewRecorder()
		server.public.Handler.ServeHTTP(rec, req)
		if rec.Code != 401 {
			t.Fatalf("unauthenticated: %d", rec.Code)
		}
		for _, policy := range []ports.WorkspaceAuthorizer{nil, readPolicy{hidden: workspaces[0]}} {
			req = httptest.NewRequest("GET", base+suffix, nil)
			req = req.WithContext(context.WithValue(req.Context(), accessContextKey{}, requestAccess{identity: security.Identity{TenantID: tenant, Principal: cfg.Auth.Principal}, authorizer: policy}))
			rec = httptest.NewRecorder()
			handler.ServeHTTP(rec, req)
			if rec.Code != 403 {
				t.Fatalf("denied %s: %d %s", suffix, rec.Code, rec.Body.String())
			}
		}
	}
	get(otherBase+"/components/"+components[0].String(), 404)
	get(otherBase+"/generations/1", 404)
	get(base+"/components/"+uuid.Must(uuid.NewV7()).String(), 404)
	get(base+"/generations/999", 404)
	for _, suffix := range []string{"/components/not-a-uuid", "/components/" + uuid.New().String(), "/generations/0", "/generations/-1", "/generations/nope"} {
		get(base+suffix, 400)
	}
	for _, table := range []string{"audit_events", "outbox_messages", "idempotency_records"} {
		var count int
		if err := db.QueryRowContext(t.Context(), "SELECT count(*) FROM thinkpixelws."+table).Scan(&count); err != nil || count != 0 {
			t.Fatalf("read mutated %s: %d %v", table, count, err)
		}
	}
	t.Log("live HTTP component/generation reads, head metadata, tenant/workspace isolation, policy, pagination, cursor scope/expiry, and no mutation verified")
}
