package httpserver

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	clockadapter "github.com/bdobrica/ThinkPixelWS/internal/adapters/clock"
	"github.com/bdobrica/ThinkPixelWS/internal/app/workspace"
	"github.com/bdobrica/ThinkPixelWS/internal/domain"
	"github.com/bdobrica/ThinkPixelWS/internal/ports"
	"github.com/bdobrica/ThinkPixelWS/internal/security"
	"github.com/google/uuid"
)

type creationStore struct{ calls []ports.WorkspaceCreation }

func (s *creationStore) CreateWorkspace(_ context.Context, in ports.WorkspaceCreation) (domain.Workspace, error) {
	s.calls = append(s.calls, in)
	return in.Workspace, nil
}

func TestCreateWorkspaceHTTP(t *testing.T) {
	cfg, token := developmentConfig(t)
	store := &creationStore{}
	api, err := NewWorkspaceAPI(workspace.Creator{Store: store, Clock: clockadapter.System{}})
	if err != nil {
		t.Fatal(err)
	}
	server, err := New(cfg, dependencies(api))
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, body, key, token string
		status                 int
	}{
		{"valid", `{"name":"demo","owner":{"kind":"user","id":"alice"}}`, "create-demo-00001", token, 201},
		{"no auth", `{}`, "create-demo-00001", "", 401},
		{"no key", `{}`, "", token, 400},
		{"bad name", `{"name":"BAD","owner":{"kind":"user","id":"alice"}}`, "create-demo-00001", token, 400},
		{"bad owner", `{"name":"demo","owner":{"kind":"root","id":"alice"}}`, "create-demo-00001", token, 400},
		{"bad class", `{"name":"demo","owner":{"kind":"user","id":"alice"},"classification":"root"}`, "create-demo-00001", token, 400},
		{"null class", `{"name":"demo","owner":{"kind":"user","id":"alice"},"classification":null}`, "create-demo-00001", token, 400},
		{"tenant injection", `{"name":"demo","owner":{"kind":"user","id":"alice"},"tenantId":"other"}`, "create-demo-00001", token, 400},
		{"duplicate residency", `{"name":"demo","owner":{"kind":"user","id":"alice"},"residency":["a","a"]}`, "create-demo-00001", token, 400},
		{"trailing JSON", `{"name":"demo","owner":{"kind":"user","id":"alice"}} {}`, "create-demo-00001", token, 400},
		{"oversized", `{"name":"demo","owner":{"kind":"user","id":"` + strings.Repeat("a", int(MaxBodyBytes)) + `"}}`, "create-demo-00001", token, 413},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest("POST", "/v1/workspaces", strings.NewReader(tc.body))
			req.RemoteAddr = "127.0.0.1:1234"
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Idempotency-Key", tc.key)
			req.Header.Set("Authorization", "Bearer "+tc.token)
			req.Header.Set("X-Tenant-ID", "attacker")
			rec := httptest.NewRecorder()
			server.public.Handler.ServeHTTP(rec, req)
			if rec.Code != tc.status {
				t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
			}
			if tc.status == 201 {
				var body map[string]any
				if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
					t.Fatal(err)
				}
				id, err := uuid.Parse(body["id"].(string))
				if err != nil || id.Version() != 7 {
					t.Fatal("invalid generated ID")
				}
				if body["state"] != "CREATING" || body["stateVersion"] != float64(1) || body["classification"] != "internal" || body["headGeneration"] != nil {
					t.Fatalf("unexpected response: %v", body)
				}
				if rec.Header().Get("Location") != "/v1/workspaces/"+id.String() {
					t.Fatal("missing Location")
				}
			} else if !strings.HasPrefix(rec.Header().Get("Content-Type"), "application/problem+json") {
				t.Fatal("not a problem response")
			}
		})
	}
	if len(store.calls) != 1 {
		t.Fatalf("persist calls: %d", len(store.calls))
	}
	if store.calls[0].Workspace.TenantID.String() != cfg.Auth.TenantID || store.calls[0].Principal != cfg.Auth.Principal {
		t.Fatal("untrusted tenant or principal")
	}
}

func TestCreateWorkspaceDeniedBeforePersistence(t *testing.T) {
	store := &creationStore{}
	api, err := NewWorkspaceAPI(workspace.Creator{Store: store, Clock: clockadapter.System{}})
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest("POST", "/v1/workspaces", strings.NewReader(`{"name":"demo","owner":{"kind":"user","id":"alice"}}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", "create-demo-00001")
	req = req.WithContext(context.WithValue(req.Context(), accessContextKey{}, requestAccess{identity: security.Identity{TenantID: uuid.MustParse("0198cb17-3520-7000-8000-000000000001"), Principal: "alice"}}))
	rec := httptest.NewRecorder()
	api.ServeHTTP(rec, req)
	if rec.Code != 403 || len(store.calls) != 0 {
		t.Fatalf("denial failed: %d %s", rec.Code, rec.Body.String())
	}
}
