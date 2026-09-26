package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLoadDefaults(t *testing.T) {
	cfg, err := load("", func(string) (string, bool) { return "", false })
	if err != nil {
		t.Fatal(err)
	}
	if cfg.HTTP.ListenAddress != "127.0.0.1:8080" {
		t.Fatalf("unexpected address %q", cfg.HTTP.ListenAddress)
	}
	if cfg.HTTP.RequestTimeout != 30*time.Second {
		t.Fatalf("unexpected timeout %s", cfg.HTTP.RequestTimeout)
	}
	if cfg.HTTP.MaxHeaderBytes != 64*1024 {
		t.Fatalf("unexpected header limit %d", cfg.HTTP.MaxHeaderBytes)
	}
}

func TestFileAndEnvironmentPrecedence(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	data := `{"http":{"listen_address":"127.0.0.1:8000","request_timeout":"20s"},"log":{"level":"warn"},"secret_references":{"database-password":{"provider":"kubernetes","reference":"namespace/name#password"}}}`
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
	env := map[string]string{envListenAddress: "0.0.0.0:8081", envRequest: "25s", envLogLevel: "error"}
	cfg, err := load(path, func(key string) (string, bool) { value, ok := env[key]; return value, ok })
	if err != nil {
		t.Fatal(err)
	}
	if cfg.HTTP.ListenAddress != "0.0.0.0:8081" || cfg.HTTP.RequestTimeout != 25*time.Second || cfg.Log.Level != "error" {
		t.Fatalf("environment did not override file: %#v", cfg)
	}
	if cfg.SecretReferences["database-password"].Provider != "kubernetes" {
		t.Fatal("secret reference was not loaded")
	}
}

func TestLoadRejectsUnknownAndInvalidValues(t *testing.T) {
	tests := []string{
		`{"unknown":true}`,
		`{"http":{"request_timeout":"forever"}}`,
		`{"secret_references":{"DB":{"provider":"env","reference":"PASSWORD"}}}`,
	}
	for _, data := range tests {
		t.Run(data, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config.json")
			if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
				t.Fatal(err)
			}
			if _, err := load(path, func(string) (string, bool) { return "", false }); err == nil {
				t.Fatal("expected error")
			}
		})
	}
}

func TestEnvironmentSecretReferencesAreLocators(t *testing.T) {
	env := map[string]string{envSecretReferences: `{"portable-key":{"provider":"vault","reference":"kv/ws/portable"}}`}
	cfg, err := load("", func(key string) (string, bool) { value, ok := env[key]; return value, ok })
	if err != nil {
		t.Fatal(err)
	}
	encoded := cfg.SecretReferences["portable-key"]
	if encoded.Reference != "kv/ws/portable" {
		t.Fatalf("unexpected reference %#v", encoded)
	}
}

func TestDevelopmentAuthConfiguration(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	data := `{"auth":{"mode":"development","tenant_id":"0198cb17-3520-7000-8000-000000000001","principal":"developer","token_file":"/local/token"}}`
	if err := os.WriteFile(path, []byte(data), 0600); err != nil {
		t.Fatal(err)
	}
	cfg, err := load(path, func(key string) (string, bool) { return "override", key == "THINKPIXELWS_AUTH_PRINCIPAL" })
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Auth.Principal != "override" || cfg.Auth.Mode != "development" || cfg.Auth.TokenFile != "/local/token" {
		t.Fatalf("config = %+v", cfg.Auth)
	}
	for _, address := range []string{"0.0.0.0:8080", "[::]:8080", "localhost:8080", "192.0.2.1:8080"} {
		bad := cfg
		bad.HTTP.ListenAddress = address
		if bad.Validate() == nil {
			t.Fatalf("unsafe bind accepted: %s", address)
		}
		bad = cfg
		bad.Metrics.ListenAddress = address
		if bad.Validate() == nil {
			t.Fatalf("unsafe metrics bind accepted: %s", address)
		}
	}
	for _, mode := range []string{"", "dev", "disabled", "oidc"} {
		bad := cfg
		bad.Auth.Mode = mode
		if bad.Validate() == nil {
			t.Fatalf("ambiguous mode accepted: %s", mode)
		}
	}
	cfg.Auth.TokenFile = ""
	if cfg.Validate() == nil {
		t.Fatal("missing credential locator accepted")
	}
	if Defaults().Auth.Mode != "disabled" {
		t.Fatal("development auth enabled by default")
	}
}
