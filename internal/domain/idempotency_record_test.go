package domain

import (
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/bdobrica/ThinkPixelWS/internal/domain/shared"
)

func TestNewIdempotencyRecord(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 9, 4, 12, 0, 0, 0, time.FixedZone("test", 7200))
	record, err := validNewIdempotencyRecord(t, now).IdempotencyRecord(now)
	if err != nil {
		t.Fatalf("create record: %v", err)
	}
	if record.Status != IdempotencyStatusInProgress || record.ResponseStatus != nil || record.ResultRef != "" {
		t.Fatalf("unexpected initial state: %+v", record)
	}
	if record.CreatedAt.Location() != time.UTC || record.ExpiresAt.Location() != time.UTC {
		t.Fatal("timestamps were not normalized to UTC")
	}
}

func TestIdempotencyRecordValidation(t *testing.T) {
	t.Parallel()
	now := time.Now().UTC()
	tests := map[string]func(*IdempotencyRecord){
		"non-v7 tenant":           func(record *IdempotencyRecord) { record.TenantID = uuid.New() },
		"blank principal":         func(record *IdempotencyRecord) { record.Principal = "" },
		"invalid operation":       func(record *IdempotencyRecord) { record.Operation = "workspace\ncreate" },
		"missing key hash":        func(record *IdempotencyRecord) { record.KeyHash = shared.SHA256Digest{} },
		"missing request digest":  func(record *IdempotencyRecord) { record.RequestDigest = shared.SHA256Digest{} },
		"expired at creation":     func(record *IdempotencyRecord) { record.ExpiresAt = record.CreatedAt },
		"updated before creation": func(record *IdempotencyRecord) { record.UpdatedAt = record.CreatedAt.Add(-time.Second) },
		"unknown status":          func(record *IdempotencyRecord) { record.Status = "UNKNOWN" },
		"in-progress result":      func(record *IdempotencyRecord) { status := 201; record.ResponseStatus = &status },
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			record, err := validNewIdempotencyRecord(t, now).IdempotencyRecord(now)
			if err != nil {
				t.Fatalf("fixture: %v", err)
			}
			mutate(&record)
			if err := record.Validate(); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}

func TestCompletedIdempotencyRecordValidation(t *testing.T) {
	t.Parallel()
	now := time.Now().UTC()
	record, err := validNewIdempotencyRecord(t, now).IdempotencyRecord(now)
	if err != nil {
		t.Fatalf("fixture: %v", err)
	}
	status := 201
	record.Status, record.ResponseStatus, record.ResultRef = IdempotencyStatusCompleted, &status, "workspaces/01991f3e-58d0-7000-8000-000000000001"
	if err := record.Validate(); err != nil {
		t.Fatalf("validate completed record: %v", err)
	}
	record.ResultRef = ""
	if err := record.Validate(); err == nil {
		t.Fatal("expected missing result reference rejection")
	}
	record.ResultRef = strings.Repeat("x", maxIdempotencyResultRefLength+1)
	if err := record.Validate(); err == nil {
		t.Fatal("expected oversized result reference rejection")
	}
}

func validNewIdempotencyRecord(t *testing.T, now time.Time) NewIdempotencyRecord {
	t.Helper()
	tid, err := uuid.NewV7()
	if err != nil {
		t.Fatalf("tenant UUID: %v", err)
	}
	return NewIdempotencyRecord{
		TenantID: tid, Principal: "principal:user-123", Operation: "workspace.create",
		KeyHash:       shared.DigestBytes([]byte("idempotency-key")),
		RequestDigest: shared.DigestBytes([]byte("canonical-request")), ExpiresAt: now.Add(24 * time.Hour),
	}
}
