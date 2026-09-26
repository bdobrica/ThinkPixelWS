package httpserver

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	clockadapter "github.com/bdobrica/ThinkPixelWS/internal/adapters/clock"
	"github.com/bdobrica/ThinkPixelWS/internal/adapters/postgres"
	"github.com/bdobrica/ThinkPixelWS/internal/app/workspace"
	"github.com/google/uuid"
	_ "github.com/lib/pq"
)

// The supplied PostgreSQL account needs CREATEDB. Every run creates and drops
// its own database; migrations never run against the supplied database itself.
func TestCreateWorkspacePostgres(t *testing.T) {
	dsn := os.Getenv("THINKPIXELWS_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set THINKPIXELWS_TEST_DATABASE_URL for isolated PostgreSQL integration")
	}
	admin, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { admin.Close() })
	name := "api001_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if _, err = admin.ExecContext(t.Context(), "CREATE DATABASE "+name); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := admin.ExecContext(context.Background(), "DROP DATABASE "+name+" WITH (FORCE)"); err != nil {
			t.Error(err)
		}
	})
	u, err := url.Parse(dsn)
	if err != nil {
		t.Fatal(err)
	}
	u.Path = "/" + name
	db, err := sql.Open("postgres", u.String())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	migrations, err := filepath.Glob("../../../migrations/*.up.sql")
	if err != nil || len(migrations) == 0 {
		t.Fatal("migrations not found")
	}
	for _, path := range migrations {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = db.ExecContext(t.Context(), string(data)); err != nil {
			t.Fatalf("%s: %v", path, err)
		}
	}
	cfg, token := developmentConfig(t)
	if _, err = db.ExecContext(t.Context(), `INSERT INTO thinkpixelws.tenants (tenant_id,lifecycle_state) VALUES ($1,'ACTIVE')`, cfg.Auth.TenantID); err != nil {
		t.Fatal(err)
	}
	api, err := NewWorkspaceAPI(workspace.Creator{Store: postgres.WorkspaceCreator{DB: db}, Clock: clockadapter.System{}})
	if err != nil {
		t.Fatal(err)
	}
	server, err := New(cfg, dependencies(api))
	if err != nil {
		t.Fatal(err)
	}
	call := func(key, body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest("POST", "/v1/workspaces", strings.NewReader(body))
		req.RemoteAddr = "127.0.0.1:1234"
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Idempotency-Key", key)
		rec := httptest.NewRecorder()
		server.public.Handler.ServeHTTP(rec, req)
		return rec
	}
	body := `{"name":"demo","owner":{"kind":"user","id":"alice"},"residency":["b","a"]}`
	const workers = 8
	responses := make([]*httptest.ResponseRecorder, workers)
	var wg sync.WaitGroup
	for i := range responses {
		wg.Add(1)
		go func() { defer wg.Done(); responses[i] = call("concurrent-create-001", body) }()
	}
	wg.Wait()
	for i, rec := range responses {
		if rec.Code != 201 {
			t.Fatalf("request %d: %d %s", i, rec.Code, rec.Body.String())
		}
		if rec.Body.String() != responses[0].Body.String() || rec.Header().Get("Location") != responses[0].Header().Get("Location") {
			t.Fatal("concurrent replay differs")
		}
	}
	var created struct {
		ID string `json:"id"`
	}
	if err = json.Unmarshal(responses[0].Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	assertCounts := func(want int) {
		t.Helper()
		for _, table := range []string{"workspaces", "audit_events", "outbox_messages", "idempotency_records"} {
			var n int
			if err := db.QueryRowContext(t.Context(), "SELECT count(*) FROM thinkpixelws."+table).Scan(&n); err != nil || n != want {
				t.Fatalf("%s count %d, want %d: %v", table, n, want, err)
			}
		}
	}
	assertCounts(1)
	var linked bool
	err = db.QueryRowContext(t.Context(), `SELECT a.transaction_id=o.transaction_id AND a.workspace_id=o.aggregate_id AND a.actor_principal=$1 FROM thinkpixelws.audit_events a JOIN thinkpixelws.outbox_messages o ON a.tenant_id=o.tenant_id`, cfg.Auth.Principal).Scan(&linked)
	if err != nil || !linked {
		t.Fatalf("audit/outbox not linked: %v", err)
	}
	// Replay is stable even after a subsequent lifecycle update and DB reconnect.
	if _, err = db.ExecContext(t.Context(), `UPDATE thinkpixelws.workspaces SET lifecycle_state='READY',state_version=2 WHERE workspace_id=$1`, created.ID); err != nil {
		t.Fatal(err)
	}
	db.SetMaxIdleConns(0)
	normalized := `{"residency":["a","b"],"classification":"internal","owner":{"id":"alice","kind":"user"},"name":"demo"}`
	replay := call("concurrent-create-001", normalized)
	if replay.Code != 201 || replay.Body.String() != responses[0].Body.String() {
		t.Fatalf("normalized replay changed: %d %s", replay.Code, replay.Body.String())
	}
	conflict := call("concurrent-create-001", strings.Replace(body, "demo", "other", 1))
	if conflict.Code != 409 {
		t.Fatalf("conflict: %d %s", conflict.Code, conflict.Body.String())
	}
	assertCounts(1)
	// Force failure at the last side-effect insert: earlier writes must roll back.
	if _, err = db.ExecContext(t.Context(), `ALTER TABLE thinkpixelws.outbox_messages ADD CONSTRAINT test_reject CHECK (false) NOT VALID`); err != nil {
		t.Fatal(err)
	}
	failed := call("rollback-create-0001", body)
	if failed.Code != 503 {
		t.Fatalf("rollback request: %d %s", failed.Code, failed.Body.String())
	}
	assertCounts(1)
	if strings.Contains(failed.Body.String(), "test_reject") {
		t.Fatal("database detail leaked")
	}
	if _, err = db.ExecContext(t.Context(), `ALTER TABLE thinkpixelws.outbox_messages DROP CONSTRAINT test_reject`); err != nil {
		t.Fatal(err)
	}
	retry := call("rollback-create-0001", body)
	if retry.Code != 201 {
		t.Fatalf("retry: %d %s", retry.Code, retry.Body.String())
	}
	assertCounts(2)
	// Identical keys in other authenticated scopes create distinct Workspaces.
	creator := workspace.Creator{Store: postgres.WorkspaceCreator{DB: db}, Clock: clockadapter.System{}}
	for _, scope := range []struct{ tenant, principal string }{{cfg.Auth.TenantID, "other-principal"}, {"0198cb17-3520-7000-8000-000000000002", cfg.Auth.Principal}} {
		if _, err = db.ExecContext(t.Context(), `INSERT INTO thinkpixelws.tenants (tenant_id,lifecycle_state) VALUES ($1,'ACTIVE') ON CONFLICT DO NOTHING`, scope.tenant); err != nil {
			t.Fatal(err)
		}
		// Reuse the HTTP boundary with the alternate configured identity.
		otherCfg := cfg
		otherCfg.Auth.TenantID = scope.tenant
		otherCfg.Auth.Principal = scope.principal
		otherAPI, e := NewWorkspaceAPI(creator)
		if e != nil {
			t.Fatal(e)
		}
		otherServer, e := New(otherCfg, dependencies(otherAPI))
		if e != nil {
			t.Fatal(e)
		}
		original := server
		server = otherServer
		rec := call("concurrent-create-001", body)
		server = original
		if rec.Code != 201 || rec.Header().Get("Location") == responses[0].Header().Get("Location") {
			t.Fatalf("scope collision: %d %s", rec.Code, rec.Body.String())
		}
	}
	assertCounts(4)
	emptyResidency := call("no-residency-create-01", `{"name":"minimal","owner":{"kind":"user","id":"alice"}}`)
	if emptyResidency.Code != 201 {
		t.Fatalf("minimal create: %d %s", emptyResidency.Code, emptyResidency.Body.String())
	}
	assertCounts(5)
	t.Log(fmt.Sprintf("verified concurrent create/replay, normalized conflict, atomic rollback, and tenant/principal isolation in %s", name))
}
