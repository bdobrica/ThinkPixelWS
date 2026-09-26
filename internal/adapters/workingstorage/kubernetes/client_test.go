package kubernetes

import (
	"context"
	"encoding/pem"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	clientapi "k8s.io/client-go/tools/clientcmd/api"
)

func TestKubeconfigClient(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-token" {
			t.Error("missing bearer authentication")
		}
		if r.Header.Get("User-Agent") != "thinkpixelws" {
			t.Error("missing user agent")
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/namespaces/ws-test/persistentvolumeclaims":
			fmt.Fprint(w, `{"apiVersion":"v1","kind":"PersistentVolumeClaimList","items":[]}`)
		case "/version":
			fmt.Fprint(w, `{"gitVersion":"v1.34.0"}`)
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	ca := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: server.Certificate().Raw})
	path := filepath.Join(t.TempDir(), "config")
	config := clientapi.Config{
		Clusters:       map[string]*clientapi.Cluster{"test": {Server: server.URL, CertificateAuthorityData: ca}, "wrong": {Server: "https://127.0.0.1:1"}},
		AuthInfos:      map[string]*clientapi.AuthInfo{"test": {Token: "test-token"}},
		Contexts:       map[string]*clientapi.Context{"selected": {Cluster: "test", AuthInfo: "test", Namespace: "ignored"}, "wrong": {Cluster: "wrong"}},
		CurrentContext: "wrong",
	}
	if err := clientcmd.WriteToFile(config, path); err != nil {
		t.Fatal(err)
	}
	t.Setenv("KUBECONFIG", filepath.Join(t.TempDir(), "missing"))
	client, err := New(Config{Kubeconfig: path, Context: "selected", Namespace: "ws-test"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.Core.PersistentVolumeClaims(client.Namespace).List(context.Background(), metav1.ListOptions{}); err != nil {
		t.Fatal(err)
	}
	if version, err := client.Discovery.ServerVersion(); err != nil || version.GitVersion != "v1.34.0" {
		t.Fatalf("discovery: %v %v", version, err)
	}
	config.CurrentContext = "selected"
	if err := clientcmd.WriteToFile(config, path); err != nil {
		t.Fatal(err)
	}
	current, err := New(Config{Kubeconfig: path, Namespace: "ws-test"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := current.Discovery.ServerVersion(); err != nil {
		t.Fatal(err)
	}
	if _, err := New(Config{Kubeconfig: path, Context: "missing", Namespace: "ws-test"}); err == nil {
		t.Fatal("accepted missing context")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := client.Core.PersistentVolumeClaims(client.Namespace).List(ctx, metav1.ListOptions{}); err == nil {
		t.Fatal("canceled request succeeded")
	}
}

func TestInClusterClient(t *testing.T) {
	tokenPath := filepath.Join(t.TempDir(), "token")
	if err := os.WriteFile(tokenPath, []byte("test-token"), 0600); err != nil {
		t.Fatal(err)
	}
	rc := &rest.Config{Host: "https://127.0.0.1", Timeout: time.Second, UserAgent: "original", BearerTokenFile: tokenPath}
	calls := 0
	client, err := newClient(Config{Namespace: "ws-test"}, func() (*rest.Config, error) { calls++; return rc, nil })
	if err != nil || client == nil || calls != 1 {
		t.Fatalf("in-cluster: %v", err)
	}
	if rc.Timeout != time.Second || rc.UserAgent != "original" {
		t.Fatal("mutated source configuration")
	}
	_, err = newClient(Config{Namespace: "ws-test"}, func() (*rest.Config, error) { return nil, errors.New("secret-token") })
	if err == nil || strings.Contains(err.Error(), "secret-token") {
		t.Fatal("loader error not safely handled")
	}
}

func TestInvalidConfiguration(t *testing.T) {
	for _, cfg := range []Config{
		{}, {Namespace: "UPPER"}, {Namespace: "ws", Context: "unexpected"},
		{Namespace: "ws", Timeout: -time.Second}, {Namespace: "ws", Timeout: 6 * time.Minute},
		{Namespace: "ws", Kubeconfig: filepath.Join(t.TempDir(), "missing")},
	} {
		if _, err := newClient(cfg, func() (*rest.Config, error) { t.Fatal("unexpected in-cluster fallback"); return nil, nil }); err == nil {
			t.Fatalf("accepted %+v", cfg)
		}
	}
	path := filepath.Join(t.TempDir(), "bad-config")
	if err := os.WriteFile(path, []byte("secret-token: [invalid"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := New(Config{Namespace: "ws", Kubeconfig: path}); err == nil || strings.Contains(err.Error(), "secret-token") {
		t.Fatal("invalid kubeconfig not safely rejected")
	}
}

func TestEmptyKubeconfigDoesNotFallBack(t *testing.T) {
	path := filepath.Join(t.TempDir(), "empty")
	if err := os.WriteFile(path, nil, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := newClient(Config{Kubeconfig: path, Namespace: "ws"}, func() (*rest.Config, error) { t.Fatal("unexpected fallback"); return nil, nil }); err == nil {
		t.Fatal("accepted empty configuration")
	}
}

func TestRequestTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { <-r.Context().Done() }))
	defer server.Close()
	client, err := newClient(Config{Namespace: "ws", Timeout: 20 * time.Millisecond}, func() (*rest.Config, error) { return &rest.Config{Host: server.URL}, nil })
	if err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	_, err = client.Core.PersistentVolumeClaims(client.Namespace).List(context.Background(), metav1.ListOptions{})
	if err == nil || time.Since(start) > time.Second {
		t.Fatalf("timeout not enforced: %v", err)
	}
}
