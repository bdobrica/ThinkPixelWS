package domain

import (
	"errors"
	"time"

	"github.com/google/uuid"

	"github.com/bdobrica/ThinkPixelWS/internal/domain/shared"
)

const (
	maxIdempotencyOperationLength = 128
	maxIdempotencyResultRefLength = 512
)

type IdempotencyStatus string

const (
	IdempotencyStatusInProgress IdempotencyStatus = "IN_PROGRESS"
	IdempotencyStatusCompleted  IdempotencyStatus = "COMPLETED"
)

// IdempotencyRecord coordinates retries within one tenant, principal, and
// operation. KeyHash deliberately stores only a one-way digest of the caller's
// Idempotency-Key; ResultRef is an opaque resource reference, never content,
// credentials, a signed URL, or execution authority.
type IdempotencyRecord struct {
	TenantID       uuid.UUID
	Principal      string
	Operation      string
	KeyHash        shared.SHA256Digest
	RequestDigest  shared.SHA256Digest
	Status         IdempotencyStatus
	ResponseStatus *int
	ResultRef      string
	CreatedAt      time.Time
	UpdatedAt      time.Time
	ExpiresAt      time.Time
}

type NewIdempotencyRecord struct {
	TenantID      uuid.UUID
	Principal     string
	Operation     string
	KeyHash       shared.SHA256Digest
	RequestDigest shared.SHA256Digest
	ExpiresAt     time.Time
}

func (input NewIdempotencyRecord) IdempotencyRecord(now time.Time) (IdempotencyRecord, error) {
	record := IdempotencyRecord{
		TenantID: input.TenantID, Principal: input.Principal, Operation: input.Operation,
		KeyHash: input.KeyHash, RequestDigest: input.RequestDigest, Status: IdempotencyStatusInProgress,
		CreatedAt: now.UTC(), UpdatedAt: now.UTC(), ExpiresAt: input.ExpiresAt.UTC(),
	}
	if err := record.Validate(); err != nil {
		return IdempotencyRecord{}, err
	}
	return record, nil
}

func (record IdempotencyRecord) Validate() error {
	if !validUUIDv7(record.TenantID) {
		return errors.New("idempotency record tenant ID must be UUIDv7")
	}
	if !validBoundedSourceValue(record.Principal, maxGenerationCreatorIDLength) {
		return errors.New("idempotency record principal is invalid")
	}
	if !validBoundedSourceValue(record.Operation, maxIdempotencyOperationLength) {
		return errors.New("idempotency record operation is invalid")
	}
	if record.KeyHash == (shared.SHA256Digest{}) || record.RequestDigest == (shared.SHA256Digest{}) {
		return errors.New("idempotency key and request digests are required")
	}
	if record.CreatedAt.IsZero() || record.UpdatedAt.Before(record.CreatedAt) || !record.ExpiresAt.After(record.CreatedAt) {
		return errors.New("idempotency record timestamps are invalid")
	}
	switch record.Status {
	case IdempotencyStatusInProgress:
		if record.ResponseStatus != nil || record.ResultRef != "" {
			return errors.New("in-progress idempotency record cannot contain a result")
		}
	case IdempotencyStatusCompleted:
		if record.ResponseStatus == nil || *record.ResponseStatus < 100 || *record.ResponseStatus > 599 || !validBoundedSourceValue(record.ResultRef, maxIdempotencyResultRefLength) {
			return errors.New("completed idempotency record result is invalid")
		}
	default:
		return errors.New("idempotency record status is invalid")
	}
	return nil
}
