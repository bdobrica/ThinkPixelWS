package workspace

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"sort"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/bdobrica/ThinkPixelWS/internal/domain"
	"github.com/bdobrica/ThinkPixelWS/internal/domain/shared"
	"github.com/bdobrica/ThinkPixelWS/internal/ports"
	"github.com/bdobrica/ThinkPixelWS/internal/ports/clock"
	"github.com/google/uuid"
)

type CreateInput struct {
	Name           string
	Owner          domain.Owner
	Classification domain.Classification
	Residency      []string
}

type Creator struct {
	Store ports.WorkspaceCreator
	Clock clock.Clock
}

// Create accepts an authenticated tenant/principal after administrative create
// authorization. Owner metadata never establishes the caller's authority.
func (c Creator) Create(ctx context.Context, tenant uuid.UUID, principal, key string, input CreateInput, requestID, traceID string) (domain.Workspace, error) {
	invalid := func() (domain.Workspace, error) {
		return domain.Workspace{}, shared.NewError(shared.CodeInvalidArgument, "invalid Workspace creation request")
	}
	if utf8.RuneCountInString(key) < 16 || utf8.RuneCountInString(key) > 128 || strings.IndexFunc(key, unicode.IsControl) >= 0 {
		return invalid()
	}
	if c.Store == nil || c.Clock == nil {
		return domain.Workspace{}, shared.NewError(shared.CodeUnavailable, "Workspace persistence is not configured")
	}
	input.Residency = append([]string{}, input.Residency...)
	sort.Strings(input.Residency)
	if input.Classification == "" {
		input.Classification = domain.ClassificationInternal
	}
	id, err := uuid.NewV7()
	if err != nil {
		return domain.Workspace{}, err
	}
	// PostgreSQL stores microseconds; use the same precision for first response and replay.
	now := c.Clock.Now().UTC().Truncate(time.Microsecond)
	w, err := (domain.NewWorkspace{
		TenantID: tenant, ID: id, Name: input.Name, Owner: input.Owner,
		Classification: input.Classification, Residency: input.Residency,
	}).Workspace(now)
	if err != nil {
		return invalid()
	}
	canonical, err := json.Marshal(struct {
		Method, Route string
		Tenant        uuid.UUID
		Principal     string
		Input         CreateInput
	}{"POST", "/v1/workspaces", tenant, principal, input})
	if err != nil {
		return domain.Workspace{}, err
	}
	transactionID, err := uuid.NewV7()
	if err != nil {
		return domain.Workspace{}, err
	}
	auditID, err := uuid.NewV7()
	if err != nil {
		return domain.Workspace{}, err
	}
	eventID, err := uuid.NewV7()
	if err != nil {
		return domain.Workspace{}, err
	}
	audit, err := (domain.NewAuditEvent{
		TenantID: tenant, WorkspaceID: &id, ID: auditID, TransactionID: transactionID,
		ActorPrincipal: principal, Action: "workspace.create", TargetKind: "workspace", TargetID: id.String(),
		Decision: "allow", Outcome: "success", RequestID: requestID, TraceID: traceID,
		MetadataSchema: "workspace-created.v1", Metadata: json.RawMessage(`{}`),
	}).AuditEvent(now)
	if err != nil {
		return domain.Workspace{}, err
	}
	payload, _ := json.Marshal(struct {
		WorkspaceID uuid.UUID `json:"workspaceId"`
	}{id})
	outbox, err := (domain.NewOutboxMessage{
		TenantID: tenant, EventID: eventID, TransactionID: transactionID,
		AggregateKind: "workspace", AggregateID: id, AggregateVersion: 1, Sequence: 1,
		EventType: "workspace.thinkpixel.io/workspace.created.v1", EventVersion: 1,
		PayloadSchema: "workspace-created.v1", Payload: payload, AvailableAt: now,
	}).OutboxMessage(now)
	if err != nil {
		return domain.Workspace{}, err
	}
	result, err := c.Store.CreateWorkspace(ctx, ports.WorkspaceCreation{
		Workspace: w, Principal: principal,
		KeyHash:       shared.SHA256Digest(sha256.Sum256([]byte(key))),
		RequestDigest: shared.SHA256Digest(sha256.Sum256(canonical)), Audit: audit, Outbox: outbox,
	})
	if errors.Is(err, ports.ErrIdempotencyConflict) {
		return domain.Workspace{}, shared.NewError(shared.CodeConflict, "Idempotency-Key was used with a different request")
	}
	if err != nil {
		return domain.Workspace{}, shared.WrapError(shared.CodeUnavailable, "Workspace creation could not be persisted", err)
	}
	return result, nil
}
