package ports

import (
	"context"
	"errors"

	"github.com/google/uuid"
)

var ErrMaterializationWriterConflict = errors.New("materialization writer is not current and eligible")
var ErrWorkspaceHeadConflict = errors.New("workspace head does not match expected generation")

// MaterializationWriter identifies a writer, not an authorization grant. The
// caller must independently authorize the operation under AG governance.
type MaterializationWriter struct {
	TenantID, WorkspaceID, MaterializationID, LeaseID uuid.UUID
	Fence                                             uint64
}

// MaterializationWriterGuard is bound to the transaction performing the protected
// metadata mutation. Success retains Workspace, lease and Materialization locks
// until that transaction ends; it must never be cached as permission for later work.
// Revalidate after provider work, immediately before recording an authoritative
// checkpoint or publishing a generation. Never hold database locks over provider IO.
// Callers must roll back on any error. Expiry/fencing cleanup is a separate operation.
type MaterializationWriterGuard interface {
	ValidateCheckpoint(context.Context, MaterializationWriter) error
	// ValidateCommit also compares the expected head. Generation creation, head
	// advancement, audit and outbox must share this serializable transaction.
	ValidateCommit(context.Context, MaterializationWriter, uint64) error
}
