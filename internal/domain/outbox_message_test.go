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

func TestNewOutboxMessage(t *testing.T) {
	t.Parallel()
	input := validNewOutboxMessage(t)
	now := time.Date(2026, time.September, 4, 18, 0, 0, 0, time.FixedZone("test", 3*60*60))
	message, err := input.OutboxMessage(now)
	if err != nil {
		t.Fatalf("create outbox message: %v", err)
	}
	if message.OccurredAt.Location() != time.UTC || message.AvailableAt.Location() != time.UTC || !message.OccurredAt.Equal(now) {
		t.Fatalf("timestamps = %v, %v", message.OccurredAt, message.AvailableAt)
	}
	input.Payload[2] = 'x'
	if bytes.Equal(message.Payload, input.Payload) {
		t.Fatal("outbox message retained mutable payload")
	}
}

func TestNewOutboxMessageRejectsInvalidInput(t *testing.T) {
	t.Parallel()
	tests := map[string]func(*NewOutboxMessage){
		"nil tenant":         func(input *NewOutboxMessage) { input.TenantID = uuid.Nil },
		"non-v7 event":       func(input *NewOutboxMessage) { input.EventID = uuid.New() },
		"nil transaction":    func(input *NewOutboxMessage) { input.TransactionID = uuid.Nil },
		"non-v7 aggregate":   func(input *NewOutboxMessage) { input.AggregateID = uuid.New() },
		"invalid aggregate":  func(input *NewOutboxMessage) { input.AggregateKind = "Workspace/type" },
		"zero version":       func(input *NewOutboxMessage) { input.AggregateVersion = 0 },
		"oversized sequence": func(input *NewOutboxMessage) { input.Sequence = math.MaxInt64 + 1 },
		"unversioned type":   func(input *NewOutboxMessage) { input.EventType = "workspace.created" },
		"zero event version": func(input *NewOutboxMessage) { input.EventVersion = 0 },
		"mismatched event version": func(input *NewOutboxMessage) {
			input.EventVersion = 2
		},
		"large event version": func(input *NewOutboxMessage) {
			input.EventVersion = math.MaxInt32 + 1
		},
		"blank schema":       func(input *NewOutboxMessage) { input.PayloadSchema = "" },
		"non-object payload": func(input *NewOutboxMessage) { input.Payload = json.RawMessage(`[]`) },
		"invalid payload":    func(input *NewOutboxMessage) { input.Payload = json.RawMessage(`{"broken"`) },
		"early availability": func(input *NewOutboxMessage) { input.AvailableAt = time.Unix(1, 0) },
		"oversized payload": func(input *NewOutboxMessage) {
			input.Payload = json.RawMessage(`{"value":"` + strings.Repeat("x", maxOutboxPayloadBytes) + `"}`)
		},
	}
	for name, mutate := range tests {
		name, mutate := name, mutate
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			input := validNewOutboxMessage(t)
			mutate(&input)
			if _, err := input.OutboxMessage(time.Now()); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}

func TestOutboxMessageValidatesDeliveryState(t *testing.T) {
	t.Parallel()
	message, err := validNewOutboxMessage(t).OutboxMessage(time.Now())
	if err != nil {
		t.Fatal(err)
	}
	message.Attempts = 2
	published := message.OccurredAt.Add(time.Second)
	message.PublishedAt = &published
	if err := message.Validate(); err != nil {
		t.Fatalf("validate published message: %v", err)
	}
	published = message.OccurredAt.Add(-time.Second)
	if err := message.Validate(); err == nil {
		t.Fatal("expected invalid publication time")
	}
	message.PublishedAt = nil
	message.Attempts = math.MaxInt32 + 1
	if err := message.Validate(); err == nil {
		t.Fatal("expected invalid delivery attempts")
	}
}

func validNewOutboxMessage(t *testing.T) NewOutboxMessage {
	t.Helper()
	newID := func() uuid.UUID {
		id, err := uuid.NewV7()
		if err != nil {
			t.Fatal(err)
		}
		return id
	}
	now := time.Now().UTC()
	return NewOutboxMessage{
		TenantID: newID(), EventID: newID(), TransactionID: newID(), AggregateKind: "workspace",
		AggregateID: newID(), AggregateVersion: 3, Sequence: 7,
		EventType: "workspace.thinkpixel.io/workspace.created.v1", EventVersion: 1,
		PayloadSchema: "workspace.created.v1", Payload: json.RawMessage(`{"state":"READY"}`), AvailableAt: now.Add(time.Minute),
	}
}
