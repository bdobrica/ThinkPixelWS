package domain

import (
	"bytes"
	"encoding/json"
	"errors"
	"math"
	"regexp"
	"time"

	"github.com/google/uuid"
)

const (
	maxWorkspaceEventTypeLength      = 128
	maxWorkspaceEventPayloadBytes    = 64 * 1024
	maxWorkspaceEventCorrelationSize = 128
)

var workspaceEventTypePattern = regexp.MustCompile(`^workspace\.thinkpixel\.io/[a-z0-9]+(?:-[a-z0-9]+)*\.[a-z0-9]+(?:-[a-z0-9]+)*\.v[1-9][0-9]*$`)

// WorkspaceEvent is immutable, ordered notification metadata for one
// Workspace. Payload is policy-safe metadata only and must never contain
// credentials, content, profile handles, signed URLs, or execution authority.
type WorkspaceEvent struct {
	TenantID         uuid.UUID
	WorkspaceID      uuid.UUID
	ID               uuid.UUID
	Sequence         uint64
	AggregateVersion uint64
	Type             string
	ActorPrincipal   string
	RunID            *uuid.UUID
	ExecutionID      *uuid.UUID
	TraceID          string
	RequestID        string
	PayloadSchema    string
	Payload          json.RawMessage
	OccurredAt       time.Time
}

type NewWorkspaceEvent struct {
	TenantID         uuid.UUID
	WorkspaceID      uuid.UUID
	ID               uuid.UUID
	Sequence         uint64
	AggregateVersion uint64
	Type             string
	ActorPrincipal   string
	RunID            *uuid.UUID
	ExecutionID      *uuid.UUID
	TraceID          string
	RequestID        string
	PayloadSchema    string
	Payload          json.RawMessage
}

func (input NewWorkspaceEvent) WorkspaceEvent(now time.Time) (WorkspaceEvent, error) {
	event := WorkspaceEvent{
		TenantID: input.TenantID, WorkspaceID: input.WorkspaceID, ID: input.ID,
		Sequence: input.Sequence, AggregateVersion: input.AggregateVersion, Type: input.Type,
		ActorPrincipal: input.ActorPrincipal, RunID: cloneUUID(input.RunID),
		ExecutionID: cloneUUID(input.ExecutionID), TraceID: input.TraceID, RequestID: input.RequestID,
		PayloadSchema: input.PayloadSchema, Payload: bytes.Clone(input.Payload), OccurredAt: now.UTC(),
	}
	if err := event.Validate(); err != nil {
		return WorkspaceEvent{}, err
	}
	return event, nil
}

func (event WorkspaceEvent) Validate() error {
	if event.TenantID == uuid.Nil || event.WorkspaceID == uuid.Nil || event.ID == uuid.Nil {
		return errors.New("Workspace event tenant, Workspace, and event IDs are required")
	}
	if event.TenantID.Version() != 7 || event.WorkspaceID.Version() != 7 || event.ID.Version() != 7 {
		return errors.New("Workspace event tenant, Workspace, and event IDs must be UUIDv7")
	}
	if event.Sequence < 1 || event.Sequence > math.MaxInt64 || event.AggregateVersion < 1 || event.AggregateVersion > math.MaxInt64 {
		return errors.New("Workspace event sequence and aggregate version are outside the supported range")
	}
	if !validBoundedSourceValue(event.Type, maxWorkspaceEventTypeLength) || !workspaceEventTypePattern.MatchString(event.Type) {
		return errors.New("Workspace event type is invalid")
	}
	if !validBoundedSourceValue(event.ActorPrincipal, maxGenerationCreatorIDLength) {
		return errors.New("Workspace event actor principal is invalid")
	}
	if !validOptionalUUIDv7(event.RunID) || !validOptionalUUIDv7(event.ExecutionID) {
		return errors.New("Workspace event Run and Execution IDs must be UUIDv7")
	}
	if !validOptionalBoundedValue(event.TraceID, maxWorkspaceEventCorrelationSize) || !validOptionalBoundedValue(event.RequestID, maxWorkspaceEventCorrelationSize) {
		return errors.New("Workspace event correlation reference is invalid")
	}
	if !validBoundedSourceValue(event.PayloadSchema, maxWorkspaceEventTypeLength) {
		return errors.New("Workspace event payload schema is invalid")
	}
	trimmedPayload := bytes.TrimSpace(event.Payload)
	if len(event.Payload) > maxWorkspaceEventPayloadBytes || !json.Valid(trimmedPayload) || len(trimmedPayload) == 0 || trimmedPayload[0] != '{' {
		return errors.New("Workspace event payload must be a bounded JSON object")
	}
	if event.OccurredAt.IsZero() {
		return errors.New("Workspace event occurrence time is required")
	}
	return nil
}

func validOptionalUUIDv7(id *uuid.UUID) bool {
	return id == nil || (*id != uuid.Nil && id.Version() == 7)
}

func validOptionalBoundedValue(value string, maximum int) bool {
	return value == "" || validBoundedSourceValue(value, maximum)
}

func cloneUUID(id *uuid.UUID) *uuid.UUID {
	if id == nil {
		return nil
	}
	copy := *id
	return &copy
}
