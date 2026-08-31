package telemetry

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"
)

func TestCorrelationUsesCanonicalFieldsAndOmitsEmptyValues(t *testing.T) {
	var output bytes.Buffer
	logger := WithCorrelation(NewJSONLogger(&output, slog.LevelDebug), Correlation{
		Tenant: "tenant-1", WorkspaceID: "ws-1", Generation: "7",
		MaterializationID: "mat-1", RequestID: "req-1", TraceID: "trace-1",
	})
	logger.Info("created")

	record := decodeRecord(t, output.String())
	for key, want := range map[string]string{
		"tenant": "tenant-1", "workspace_id": "ws-1", "generation": "7",
		"materialization_id": "mat-1", "request_id": "req-1", "trace_id": "trace-1",
	} {
		if got := record[key]; got != want {
			t.Errorf("%s = %v, want %q", key, got, want)
		}
	}
	if _, exists := record["run_id"]; exists {
		t.Error("empty run_id was logged")
	}
}

func TestRedactionIsRecursiveForRecordAndLoggerAttributes(t *testing.T) {
	type credentials struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	var output bytes.Buffer
	logger := NewJSONLogger(&output, slog.LevelInfo).With("source", map[string]any{
		"repository": "repo-1",
		"auth":       map[string]any{"access_token": "nested-token", "mode": "grant"},
	})
	logger.Info("import", "request", map[string]any{
		"headers":     map[string]string{"Authorization": "Bearer abc", "Accept": "application/json"},
		"credentials": credentials{Username: "user", Password: "hunter2"},
		"items":       []any{map[string]any{"signed_url": "https://secret", "apiToken": "camel-secret", "status": "ready"}},
	})

	line := output.String()
	for _, secret := range []string{"nested-token", "Bearer abc", "hunter2", "https://secret", "camel-secret"} {
		if strings.Contains(line, secret) {
			t.Errorf("log contains secret %q: %s", secret, line)
		}
	}
	if !strings.Contains(line, Redacted) || !strings.Contains(line, "repo-1") || !strings.Contains(line, "ready") {
		t.Fatalf("redaction removed safe context or did not mark secrets: %s", line)
	}
}

func TestSlogGroupsAreRecursivelyRedacted(t *testing.T) {
	var output bytes.Buffer
	logger := NewJSONLogger(&output, slog.LevelInfo)
	logger.LogAttrs(context.Background(), slog.LevelInfo, "request",
		slog.Group("http", slog.String("cookie", "session=abc"), slog.Int("status", 200)),
		slog.String("response_body", "sensitive payload"),
	)
	line := output.String()
	if strings.Contains(line, "session=abc") || strings.Contains(line, "sensitive payload") {
		t.Fatalf("group or direct secret was not redacted: %s", line)
	}
	if !strings.Contains(line, `"status":200`) {
		t.Fatalf("safe group field missing: %s", line)
	}
}

func TestRedactionHandlesCycles(t *testing.T) {
	value := map[string]any{"password": "secret"}
	value["self"] = value
	var output bytes.Buffer
	NewJSONLogger(&output, slog.LevelInfo).Info("cycle", "value", value)
	if strings.Contains(output.String(), "secret") {
		t.Fatalf("cycle leaked secret: %s", output.String())
	}
}

func TestRedactionDoesNotInvokeArbitraryStringers(t *testing.T) {
	var output bytes.Buffer
	NewJSONLogger(&output, slog.LevelInfo).Info("value", "object", secretStringer{Password: "stringer-secret"})
	if strings.Contains(output.String(), "stringer-secret") {
		t.Fatalf("String method leaked a secret: %s", output.String())
	}
}

type secretStringer struct{ Password string }

func (s secretStringer) String() string { return s.Password }

func decodeRecord(t *testing.T, line string) map[string]any {
	t.Helper()
	var record map[string]any
	if err := json.Unmarshal([]byte(line), &record); err != nil {
		t.Fatalf("decode log record: %v\n%s", err, line)
	}
	return record
}
