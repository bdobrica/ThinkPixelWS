package httpserver

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bdobrica/ThinkPixelWS/internal/config"
	"github.com/bdobrica/ThinkPixelWS/internal/ports"
	"github.com/google/uuid"
)

func developmentConfig(t *testing.T) (config.Config, string) {
	t.Helper()
	token := strings.Repeat("fixture-token", 4)
	path := filepath.Join(t.TempDir(), "token")
	if err := os.WriteFile(path, []byte(token+"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	cfg := config.Defaults()
	cfg.Auth = config.AuthConfig{Mode: "development", TenantID: "0198cb17-3520-7000-8000-000000000001", Principal: "developer", TokenFile: path}
	return cfg, token
}

func TestDevelopmentHTTPBoundary(t *testing.T) {
	cfg, token := developmentConfig(t)
	calls := 0
	server, err := New(cfg, dependencies(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		identity, ok := IdentityFromContext(r.Context())
		if !ok || identity.TenantID.String() != cfg.Auth.TenantID || identity.Principal != cfg.Auth.Principal {
			t.Fatal("configured identity not preserved")
		}
		if err := AuthorizeWorkspace(r.Context(), ports.WorkspaceCreate, uuid.Nil); err != nil {
			t.Fatal(err)
		}
		if err := AuthorizeWorkspace(r.Context(), ports.WorkspaceRequestMaterialization, identity.TenantID); err == nil {
			t.Fatal("materialization allowed")
		}
		w.WriteHeader(http.StatusNoContent)
	})))
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, peer string
		headers    []string
		status     int
	}{
		{"valid", "127.0.0.1:1234", []string{"Bearer " + token}, 204},
		{"ipv6", "[::1]:1234", []string{"Bearer " + token}, 204},
		{"missing", "127.0.0.1:1234", nil, 401},
		{"wrong", "127.0.0.1:1234", []string{"Bearer wrong"}, 401},
		{"duplicate", "127.0.0.1:1234", []string{"Bearer " + token, "Bearer " + token}, 401},
		{"remote", "192.0.2.1:1234", []string{"Bearer " + token}, 401},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest("POST", "/v1/workspaces", nil)
			req.RemoteAddr = tc.peer
			for _, value := range tc.headers {
				req.Header.Add("Authorization", value)
			}
			req.Header.Set("X-Forwarded-For", "127.0.0.1")
			req.Header.Set("X-Tenant-ID", "attacker")
			req.Header.Set("X-Principal", "attacker")
			rec := httptest.NewRecorder()
			server.public.Handler.ServeHTTP(rec, req)
			if rec.Code != tc.status {
				t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
			}
			if strings.Contains(rec.Body.String(), token) {
				t.Fatal("token leaked")
			}
		})
	}
	if calls != 2 {
		t.Fatalf("API calls = %d", calls)
	}
	for _, path := range []string{"/livez", "/readyz"} {
		rec := httptest.NewRecorder()
		server.public.Handler.ServeHTTP(rec, httptest.NewRequest("GET", path, nil))
		if rec.Code != 200 {
			t.Fatalf("%s: %d", path, rec.Code)
		}
	}
	if _, ok := IdentityFromContext(context.Background()); ok {
		t.Fatal("ambient identity")
	}
	if AuthorizeWorkspace(context.Background(), ports.WorkspaceList, uuid.Nil) == nil {
		t.Fatal("ambient authorization")
	}
}

func TestDevelopmentStartupAndDisabledMode(t *testing.T) {
	cfg, _ := developmentConfig(t)
	for _, mutate := range []func(*config.Config){
		func(c *config.Config) { c.HTTP.ListenAddress = "0.0.0.0:8080" },
		func(c *config.Config) { c.Auth.TenantID = "invalid" },
		func(c *config.Config) { c.Auth.Principal = " developer " },
		func(c *config.Config) { c.Auth.TokenFile = filepath.Join(t.TempDir(), "missing") },
		func(c *config.Config) { c.Auth.TokenFile = t.TempDir() },
	} {
		bad := cfg
		mutate(&bad)
		if _, err := New(bad, dependencies(nil)); err == nil {
			t.Fatal("unsafe configuration accepted")
		}
	}
	if err := os.WriteFile(cfg.Auth.TokenFile, []byte("short"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := New(cfg, dependencies(nil)); err == nil {
		t.Fatal("weak token accepted")
	}
	server, err := New(config.Defaults(), dependencies(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { t.Fatal("unconfigured API reached") })))
	if err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	server.public.Handler.ServeHTTP(rec, httptest.NewRequest("GET", "/v1/workspaces", nil))
	if rec.Code != 503 {
		t.Fatalf("disabled status = %d", rec.Code)
	}
}
