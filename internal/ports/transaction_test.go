package ports_test

import (
	"context"
	"testing"
	"time"

	"github.com/bdobrica/ThinkPixelWS/internal/domain"
	"github.com/bdobrica/ThinkPixelWS/internal/domain/shared"
	"github.com/bdobrica/ThinkPixelWS/internal/ports"
	"github.com/google/uuid"
)

// These compile-time checks keep transaction callbacks coupled to the complete
// atomic mutation surface instead of a database-specific transaction type.
var (
	_ ports.TransactionManager      = transactionManagerStub{}
	_ ports.TransactionRepositories = transactionRepositoriesStub{}
)

func TestTransactionManagerProvidesScopedRepositories(t *testing.T) {
	t.Parallel()

	called := false
	err := (transactionManagerStub{}).WithinTransaction(t.Context(), func(repositories ports.TransactionRepositories) error {
		called = true
		if repositories == nil {
			t.Fatal("transaction repositories must be provided")
		}
		return nil
	})
	if err != nil {
		t.Fatalf("run transaction callback: %v", err)
	}
	if !called {
		t.Fatal("transaction callback was not called")
	}
}

type transactionManagerStub struct{}

func (transactionManagerStub) WithinTransaction(_ context.Context, operation func(ports.TransactionRepositories) error) error {
	return operation(transactionRepositoriesStub{})
}

type transactionRepositoriesStub struct{}

func (transactionRepositoriesStub) Create(context.Context, uuid.UUID, domain.Workspace) error {
	return nil
}

func (transactionRepositoriesStub) Get(context.Context, uuid.UUID, uuid.UUID) (domain.Workspace, error) {
	return domain.Workspace{}, nil
}

func (transactionRepositoriesStub) TransitionState(context.Context, uuid.UUID, uuid.UUID, domain.WorkspaceState, domain.WorkspaceState, uint64, time.Time) error {
	return nil
}

func (transactionRepositoriesStub) AppendWorkspaceEvent(context.Context, uuid.UUID, domain.WorkspaceEvent) error {
	return nil
}

func (transactionRepositoriesStub) AppendAuditEvent(context.Context, uuid.UUID, domain.AuditEvent) error {
	return nil
}

func (transactionRepositoriesStub) CreateIdempotencyRecord(context.Context, uuid.UUID, domain.IdempotencyRecord) error {
	return nil
}

func (transactionRepositoriesStub) GetIdempotencyRecord(context.Context, uuid.UUID, string, string, shared.SHA256Digest) (domain.IdempotencyRecord, error) {
	return domain.IdempotencyRecord{}, nil
}

func (transactionRepositoriesStub) CompleteIdempotencyRecord(context.Context, uuid.UUID, domain.IdempotencyRecord) error {
	return nil
}

func (transactionRepositoriesStub) EnqueueOutboxMessage(context.Context, uuid.UUID, domain.OutboxMessage) error {
	return nil
}

func (transactionRepositoriesStub) AdvanceWriterFence(context.Context, uuid.UUID, uuid.UUID) (uint64, error) {
	return 1, nil
}
