package domain

import (
	"bytes"
	"encoding/json"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestNewWorkspaceEvent(t *testing.T) {
	t.Parallel()

	input := validNewWorkspaceEvent(t)
	runID := *input.RunID
	executionID := *input.ExecutionID
	now := time.Date(2026, time.September, 4, 12, 0, 0, 0, time.FixedZone("test", 2*60*60))
	event, err := input.WorkspaceEvent(now)
	if err != nil {
		t.Fatalf("create Workspace event: %v", err)
	}
	if event.OccurredAt.Location() != time.UTC || !event.OccurredAt.Equal(now) {
		t.Fatalf("occurrence time = %v", event.OccurredAt)
	}

	input.Payload[2] = 'x'
	*input.RunID = uuid.Nil
	*input.ExecutionID = uuid.Nil
	if bytes.Equal(event.Payload, input.Payload) || *event.RunID != runID || *event.ExecutionID != executionID {
		t.Fatal("Workspace event retained mutable input")
	}
}

func TestNewWorkspaceEventRejectsInvalidInput(t *testing.T) {
	t.Parallel()

	tests := map[string]func(*NewWorkspaceEvent){
		"nil tenant ID":        func(input *NewWorkspaceEvent) { input.TenantID = uuid.Nil },
		"non-v7 Workspace ID":  func(input *NewWorkspaceEvent) { input.WorkspaceID = uuid.New() },
		"non-v7 event ID":      func(input *NewWorkspaceEvent) { input.ID = uuid.New() },
		"zero sequence":        func(input *NewWorkspaceEvent) { input.Sequence = 0 },
		"oversized sequence":   func(input *NewWorkspaceEvent) { input.Sequence = math.MaxInt64 + 1 },
		"zero version":         func(input *NewWorkspaceEvent) { input.AggregateVersion = 0 },
		"oversized version":    func(input *NewWorkspaceEvent) { input.AggregateVersion = math.MaxInt64 + 1 },
		"unversioned type":     func(input *NewWorkspaceEvent) { input.Type = "workspace.created" },
		"bad namespace":        func(input *NewWorkspaceEvent) { input.Type = "other.example/workspace.created.v1" },
		"blank actor":          func(input *NewWorkspaceEvent) { input.ActorPrincipal = "" },
		"control actor":        func(input *NewWorkspaceEvent) { input.ActorPrincipal = "actor\n" },
		"non-v7 Run ID":        func(input *NewWorkspaceEvent) { id := uuid.New(); input.RunID = &id },
		"non-v7 Execution ID":  func(input *NewWorkspaceEvent) { id := uuid.New(); input.ExecutionID = &id },
		"unnormalized trace":   func(input *NewWorkspaceEvent) { input.TraceID = " trace " },
		"control request":      func(input *NewWorkspaceEvent) { input.RequestID = "request\n" },
		"blank payload schema": func(input *NewWorkspaceEvent) { input.PayloadSchema = "" },
		"non-object payload":   func(input *NewWorkspaceEvent) { input.Payload = json.RawMessage(`[]`) },
		"invalid payload":      func(input *NewWorkspaceEvent) { input.Payload = json.RawMessage(`{"broken"`) },
		"oversized payload": func(input *NewWorkspaceEvent) {
			input.Payload = json.RawMessage(`{"value":"` + strings.Repeat("x", maxWorkspaceEventPayloadBytes) + `"}`)
		},
	}
	for name, mutate := range tests {
		name, mutate := name, mutate
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			input := validNewWorkspaceEvent(t)
			mutate(&input)
			if _, err := input.WorkspaceEvent(time.Now()); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}

func TestWorkspaceEventRejectsMissingOccurrenceTime(t *testing.T) {
	t.Parallel()

	event, err := validNewWorkspaceEvent(t).WorkspaceEvent(time.Now())
	if err != nil {
		t.Fatal(err)
	}
	event.OccurredAt = time.Time{}
	if err := event.Validate(); err == nil {
		t.Fatal("expected validation error")
	}
}

func TestWorkspaceEventAcceptsEmptyOptionalCorrelationsAndWhitespaceAroundPayload(t *testing.T) {
	t.Parallel()

	input := validNewWorkspaceEvent(t)
	input.RunID = nil
	input.ExecutionID = nil
	input.TraceID = ""
	input.RequestID = ""
	input.Payload = json.RawMessage(" \n {\"state\":\"READY\"} \t")
	if _, err := input.WorkspaceEvent(time.Now()); err != nil {
		t.Fatalf("create Workspace event: %v", err)
	}
}

func validNewWorkspaceEvent(t *testing.T) NewWorkspaceEvent {
	t.Helper()
	newID := func() uuid.UUID {
		id, err := uuid.NewV7()
		if err != nil {
			t.Fatal(err)
		}
		return id
	}
	runID, executionID := newID(), newID()
	return NewWorkspaceEvent{
		TenantID: newID(), WorkspaceID: newID(), ID: newID(), Sequence: 7, AggregateVersion: 3,
		Type: "workspace.thinkpixel.io/workspace.created.v1", ActorPrincipal: "principal-123",
		RunID: &runID, ExecutionID: &executionID, TraceID: "4bf92f3577b34da6a3ce929d0e0e4736",
		RequestID: "01991a75-5fc7-7f6a-83c7-b0b78fa19f01", PayloadSchema: "workspace.created.v1",
		Payload: json.RawMessage(`{"state":"READY"}`),
	}
}
