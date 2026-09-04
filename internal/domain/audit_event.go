package domain

import (
	"bytes"
	"encoding/json"
	"errors"
	"regexp"
	"time"

	"github.com/google/uuid"
)

const (
	maxAuditEventNameLength    = 128
	maxAuditEventTargetLength  = 256
	maxAuditEventMetadataBytes = 64 * 1024
)

var auditEventNamePattern = regexp.MustCompile(`^[a-z][a-z0-9]*(?:[.-][a-z0-9]+)*$`)

// AuditEvent is an immutable record of a security-relevant decision or state
// mutation. TransactionID couples it to the business transaction that caused
// it. Metadata is policy-safe only and must never contain content, credentials,
// profile handles, signed URLs, or execution authority.
type AuditEvent struct {
	TenantID       uuid.UUID
	WorkspaceID    *uuid.UUID
	ID             uuid.UUID
	TransactionID  uuid.UUID
	ActorPrincipal string
	Action         string
	TargetKind     string
	TargetID       string
	Decision       string
	Outcome        string
	TraceID        string
	RequestID      string
	MetadataSchema string
	Metadata       json.RawMessage
	OccurredAt     time.Time
}

type NewAuditEvent struct {
	TenantID       uuid.UUID
	WorkspaceID    *uuid.UUID
	ID             uuid.UUID
	TransactionID  uuid.UUID
	ActorPrincipal string
	Action         string
	TargetKind     string
	TargetID       string
	Decision       string
	Outcome        string
	TraceID        string
	RequestID      string
	MetadataSchema string
	Metadata       json.RawMessage
}

func (input NewAuditEvent) AuditEvent(now time.Time) (AuditEvent, error) {
	event := AuditEvent{
		TenantID: input.TenantID, WorkspaceID: cloneUUID(input.WorkspaceID), ID: input.ID,
		TransactionID: input.TransactionID, ActorPrincipal: input.ActorPrincipal,
		Action: input.Action, TargetKind: input.TargetKind, TargetID: input.TargetID,
		Decision: input.Decision, Outcome: input.Outcome, TraceID: input.TraceID,
		RequestID: input.RequestID, MetadataSchema: input.MetadataSchema,
		Metadata: bytes.Clone(input.Metadata), OccurredAt: now.UTC(),
	}
	if err := event.Validate(); err != nil {
		return AuditEvent{}, err
	}
	return event, nil
}

func (event AuditEvent) Validate() error {
	if !validUUIDv7(event.TenantID) || !validUUIDv7(event.ID) || !validUUIDv7(event.TransactionID) {
		return errors.New("audit event tenant, event, and transaction IDs must be UUIDv7")
	}
	if !validOptionalUUIDv7(event.WorkspaceID) {
		return errors.New("audit event Workspace ID must be UUIDv7")
	}
	if !validBoundedSourceValue(event.ActorPrincipal, maxGenerationCreatorIDLength) {
		return errors.New("audit event actor principal is invalid")
	}
	for _, value := range []string{event.Action, event.TargetKind, event.Decision, event.Outcome} {
		if !validBoundedSourceValue(value, maxAuditEventNameLength) || !auditEventNamePattern.MatchString(value) {
			return errors.New("audit event action, target kind, decision, or outcome is invalid")
		}
	}
	if !validBoundedSourceValue(event.TargetID, maxAuditEventTargetLength) {
		return errors.New("audit event target ID is invalid")
	}
	if !validOptionalBoundedValue(event.TraceID, maxWorkspaceEventCorrelationSize) || !validOptionalBoundedValue(event.RequestID, maxWorkspaceEventCorrelationSize) {
		return errors.New("audit event correlation reference is invalid")
	}
	if !validBoundedSourceValue(event.MetadataSchema, maxAuditEventNameLength) {
		return errors.New("audit event metadata schema is invalid")
	}
	trimmedMetadata := bytes.TrimSpace(event.Metadata)
	if len(event.Metadata) > maxAuditEventMetadataBytes || !json.Valid(trimmedMetadata) || len(trimmedMetadata) == 0 || trimmedMetadata[0] != '{' {
		return errors.New("audit event metadata must be a bounded JSON object")
	}
	if event.OccurredAt.IsZero() {
		return errors.New("audit event occurrence time is required")
	}
	return nil
}

func validUUIDv7(id uuid.UUID) bool {
	return id != uuid.Nil && id.Version() == 7
}
