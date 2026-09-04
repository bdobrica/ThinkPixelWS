package ports

import (
	"context"

	"github.com/bdobrica/ThinkPixelWS/internal/domain"
	"github.com/bdobrica/ThinkPixelWS/internal/domain/shared"
	"github.com/google/uuid"
)

// WorkspaceEventRepository appends immutable, tenant-scoped Workspace events.
type WorkspaceEventRepository interface {
	AppendWorkspaceEvent(context.Context, uuid.UUID, domain.WorkspaceEvent) error
}

// AuditEventRepository appends immutable, tenant-scoped audit records.
type AuditEventRepository interface {
	AppendAuditEvent(context.Context, uuid.UUID, domain.AuditEvent) error
}

// IdempotencyRepository coordinates one mutation result within its complete
// tenant, principal, operation, and key-hash scope.
type IdempotencyRepository interface {
	CreateIdempotencyRecord(context.Context, uuid.UUID, domain.IdempotencyRecord) error
	GetIdempotencyRecord(context.Context, uuid.UUID, string, string, shared.SHA256Digest) (domain.IdempotencyRecord, error)
	CompleteIdempotencyRecord(context.Context, uuid.UUID, domain.IdempotencyRecord) error
}

// OutboxRepository enqueues immutable, tenant-scoped messages for at-least-once
// delivery. Delivery attempts happen outside the originating transaction.
type OutboxRepository interface {
	EnqueueOutboxMessage(context.Context, uuid.UUID, domain.OutboxMessage) error
}

// TransactionRepositories is the transaction-scoped persistence surface used
// by security-sensitive Workspace mutations. Every method retains an explicit
// tenant argument: transaction scope does not replace tenant isolation.
//
// Implementations must bind every embedded repository to the same transaction.
// The value is valid only for the duration of a TransactionManager callback and
// must not be retained or used concurrently.
type TransactionRepositories interface {
	WorkspaceRepository
	WorkspaceEventRepository
	AuditEventRepository
	IdempotencyRepository
	OutboxRepository
}

// TransactionManager executes a callback against one atomic metadata-store
// transaction. It commits only when the callback returns nil and otherwise
// rolls back. Implementations must honor context cancellation and recover the
// transaction before propagating a callback panic.
type TransactionManager interface {
	WithinTransaction(context.Context, func(TransactionRepositories) error) error
}
