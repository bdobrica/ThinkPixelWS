package domain

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
)

const (
	maxOutboxAggregateKindLength = 128
	maxOutboxEventTypeLength     = 128
	maxOutboxPayloadBytes        = 64 * 1024
)

var outboxNamePattern = regexp.MustCompile(`^[a-z][a-z0-9]*(?:[.-][a-z0-9]+)*$`)

// OutboxMessage is a transactionally persisted event awaiting at-least-once
// delivery. Payload is policy-safe metadata only and must never contain
// content, credentials, profile handles, signed URLs, or execution authority.
// Consumers deduplicate deliveries by EventID.
type OutboxMessage struct {
	TenantID         uuid.UUID
	EventID          uuid.UUID
	TransactionID    uuid.UUID
	AggregateKind    string
	AggregateID      uuid.UUID
	AggregateVersion uint64
	Sequence         uint64
	EventType        string
	EventVersion     uint32
	PayloadSchema    string
	Payload          json.RawMessage
	Attempts         uint32
	OccurredAt       time.Time
	AvailableAt      time.Time
	PublishedAt      *time.Time
}

type NewOutboxMessage struct {
	TenantID         uuid.UUID
	EventID          uuid.UUID
	TransactionID    uuid.UUID
	AggregateKind    string
	AggregateID      uuid.UUID
	AggregateVersion uint64
	Sequence         uint64
	EventType        string
	EventVersion     uint32
	PayloadSchema    string
	Payload          json.RawMessage
	AvailableAt      time.Time
}

func (input NewOutboxMessage) OutboxMessage(now time.Time) (OutboxMessage, error) {
	message := OutboxMessage{
		TenantID: input.TenantID, EventID: input.EventID, TransactionID: input.TransactionID,
		AggregateKind: input.AggregateKind, AggregateID: input.AggregateID,
		AggregateVersion: input.AggregateVersion, Sequence: input.Sequence,
		EventType: input.EventType, EventVersion: input.EventVersion,
		PayloadSchema: input.PayloadSchema, Payload: bytes.Clone(input.Payload),
		OccurredAt: now.UTC(), AvailableAt: input.AvailableAt.UTC(),
	}
	if err := message.Validate(); err != nil {
		return OutboxMessage{}, err
	}
	return message, nil
}

func (message OutboxMessage) Validate() error {
	if !validUUIDv7(message.TenantID) || !validUUIDv7(message.EventID) ||
		!validUUIDv7(message.TransactionID) || !validUUIDv7(message.AggregateID) {
		return errors.New("outbox tenant, event, transaction, and aggregate IDs must be UUIDv7")
	}
	if !validBoundedSourceValue(message.AggregateKind, maxOutboxAggregateKindLength) || !outboxNamePattern.MatchString(message.AggregateKind) {
		return errors.New("outbox aggregate kind is invalid")
	}
	if message.AggregateVersion < 1 || message.AggregateVersion > math.MaxInt64 || message.Sequence < 1 || message.Sequence > math.MaxInt64 {
		return errors.New("outbox aggregate version and sequence are outside the supported range")
	}
	if !validBoundedSourceValue(message.EventType, maxOutboxEventTypeLength) || !workspaceEventTypePattern.MatchString(message.EventType) || message.EventVersion < 1 || message.EventVersion > math.MaxInt32 || !strings.HasSuffix(message.EventType, fmt.Sprintf(".v%d", message.EventVersion)) {
		return errors.New("outbox event type or version is invalid")
	}
	if message.Attempts > math.MaxInt32 {
		return errors.New("outbox delivery attempts are outside the supported range")
	}
	if !validBoundedSourceValue(message.PayloadSchema, maxOutboxEventTypeLength) {
		return errors.New("outbox payload schema is invalid")
	}
	trimmedPayload := bytes.TrimSpace(message.Payload)
	if len(message.Payload) > maxOutboxPayloadBytes || !json.Valid(trimmedPayload) || len(trimmedPayload) == 0 || trimmedPayload[0] != '{' {
		return errors.New("outbox payload must be a bounded JSON object")
	}
	if message.OccurredAt.IsZero() || message.AvailableAt.Before(message.OccurredAt) {
		return errors.New("outbox timestamps are invalid")
	}
	if message.PublishedAt != nil && message.PublishedAt.Before(message.OccurredAt) {
		return errors.New("outbox publication time is invalid")
	}
	return nil
}
