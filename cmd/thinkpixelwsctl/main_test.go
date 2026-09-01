package main

import (
	"context"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWorkspaceListUsesGeneratedClientAndBearerToken(t *testing.T) {
	t.Parallel()
	transport := roundTripperFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path != "/v1/workspaces" || r.URL.Query().Get("limit") != "25" {
			t.Errorf("unexpected request URL: %s", r.URL.String())
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-token" {
			t.Errorf("Authorization = %q", got)
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader(`{"items":[],"nextCursor":null}`)),
			Request:    r,
		}, nil
	})

	tokenFile := writeTokenFile(t, "test-token\n")
	client, err := newAPIClientWithTransport("http://127.0.0.1:8080", tokenFile, transport)
	if err != nil {
		t.Fatal(err)
	}
	var stdout, stderr strings.Builder
	err = runWorkspaceList(context.Background(), client, []string{"--limit", "25"}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("run() error = %v; stderr = %s", err, stderr.String())
	}
	if got := strings.TrimSpace(stdout.String()); got != `{"items":[],"nextCursor":null}` {
		t.Fatalf("stdout = %q", got)
	}
}

func TestWorkspaceDescribeValidatesIDBeforeRequest(t *testing.T) {
	t.Parallel()
	tokenFile := writeTokenFile(t, "test-token")
	err := run(context.Background(), []string{"--token-file", tokenFile, "workspace", "describe", "not-a-uuid"}, noEnvironment, io.Discard, io.Discard)
	if err == nil || !strings.Contains(err.Error(), "invalid workspace ID") {
		t.Fatalf("run() error = %v", err)
	}
}

func TestNewAPIClientRejectsRemotePlaintext(t *testing.T) {
	t.Parallel()
	_, err := newAPIClient("http://workspace.example.com", writeTokenFile(t, "test-token"))
	if err == nil || !strings.Contains(err.Error(), "must use HTTPS") {
		t.Fatalf("newAPIClient() error = %v", err)
	}
}

func TestTokenIsRequired(t *testing.T) {
	t.Parallel()
	err := run(context.Background(), []string{"workspace", "list"}, noEnvironment, io.Discard, io.Discard)
	if err == nil || !strings.Contains(err.Error(), "token file is required") {
		t.Fatalf("run() error = %v", err)
	}
}

func TestHelpDoesNotRequireToken(t *testing.T) {
	t.Parallel()
	var stdout strings.Builder
	if err := run(context.Background(), []string{"help"}, noEnvironment, &stdout, io.Discard); err != nil {
		t.Fatalf("run() error = %v", err)
	}
	if !strings.Contains(stdout.String(), "workspace describe") {
		t.Fatalf("help output = %q", stdout.String())
	}
}

func writeTokenFile(t *testing.T, token string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "token")
	if err := os.WriteFile(path, []byte(token), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func noEnvironment(string) string { return "" }

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}
