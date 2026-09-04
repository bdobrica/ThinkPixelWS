package domain

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestNewAuditEvent(t *testing.T) {
	t.Parallel()

	input := validNewAuditEvent(t)
	workspaceID := *input.WorkspaceID
	now := time.Date(2026, time.September, 4, 15, 0, 0, 0, time.FixedZone("test", 3*60*60))
	event, err := input.AuditEvent(now)
	if err != nil {
		t.Fatalf("create audit event: %v", err)
	}
	if event.OccurredAt.Location() != time.UTC || !event.OccurredAt.Equal(now) {
		t.Fatalf("occurrence time = %v", event.OccurredAt)
	}
	input.Metadata[2] = 'x'
	*input.WorkspaceID = uuid.Nil
	if bytes.Equal(event.Metadata, input.Metadata) || *event.WorkspaceID != workspaceID {
		t.Fatal("audit event retained mutable input")
	}
}

func TestNewAuditEventRejectsInvalidInput(t *testing.T) {
	t.Parallel()

	tests := map[string]func(*NewAuditEvent){
		"nil tenant ID":         func(input *NewAuditEvent) { input.TenantID = uuid.Nil },
		"non-v7 event ID":       func(input *NewAuditEvent) { input.ID = uuid.New() },
		"nil transaction ID":    func(input *NewAuditEvent) { input.TransactionID = uuid.Nil },
		"non-v7 Workspace ID":   func(input *NewAuditEvent) { id := uuid.New(); input.WorkspaceID = &id },
		"blank actor":           func(input *NewAuditEvent) { input.ActorPrincipal = "" },
		"invalid action":        func(input *NewAuditEvent) { input.Action = "Workspace Create" },
		"invalid target kind":   func(input *NewAuditEvent) { input.TargetKind = "workspace/type" },
		"blank target ID":       func(input *NewAuditEvent) { input.TargetID = "" },
		"invalid decision":      func(input *NewAuditEvent) { input.Decision = "ALLOW\n" },
		"invalid outcome":       func(input *NewAuditEvent) { input.Outcome = "" },
		"unnormalized trace":    func(input *NewAuditEvent) { input.TraceID = " trace " },
		"control request":       func(input *NewAuditEvent) { input.RequestID = "request\n" },
		"blank metadata schema": func(input *NewAuditEvent) { input.MetadataSchema = "" },
		"non-object metadata":   func(input *NewAuditEvent) { input.Metadata = json.RawMessage(`[]`) },
		"invalid metadata":      func(input *NewAuditEvent) { input.Metadata = json.RawMessage(`{"broken"`) },
		"oversized metadata": func(input *NewAuditEvent) {
			input.Metadata = json.RawMessage(`{"value":"` + strings.Repeat("x", maxAuditEventMetadataBytes) + `"}`)
		},
	}
	for name, mutate := range tests {
		name, mutate := name, mutate
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			input := validNewAuditEvent(t)
			mutate(&input)
			if _, err := input.AuditEvent(time.Now()); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}

func TestAuditEventAllowsTenantTargetAndRejectsMissingOccurrenceTime(t *testing.T) {
	t.Parallel()

	input := validNewAuditEvent(t)
	input.WorkspaceID = nil
	event, err := input.AuditEvent(time.Now())
	if err != nil {
		t.Fatal(err)
	}
	event.OccurredAt = time.Time{}
	if err := event.Validate(); err == nil {
		t.Fatal("expected validation error")
	}
}

func validNewAuditEvent(t *testing.T) NewAuditEvent {
	t.Helper()
	newID := func() uuid.UUID {
		id, err := uuid.NewV7()
		if err != nil {
			t.Fatal(err)
		}
		return id
	}
	workspaceID := newID()
	return NewAuditEvent{
		TenantID: newID(), WorkspaceID: &workspaceID, ID: newID(), TransactionID: newID(),
		ActorPrincipal: "principal-123", Action: "workspace.create", TargetKind: "workspace",
		TargetID: workspaceID.String(), Decision: "allow", Outcome: "succeeded",
		TraceID: "4bf92f3577b34da6a3ce929d0e0e4736", RequestID: "01991a75-5fc7-7f6a-83c7-b0b78fa19f01",
		MetadataSchema: "workspace.create.v1", Metadata: json.RawMessage(`{"classification":"internal"}`),
	}
}
