package domain

import (
	"errors"
	"math"
	"time"

	"github.com/google/uuid"
)

// WorkspaceFork records immutable lineage from a completed source generation
// to generation 1 of a distinct child Workspace. Lineage is historical
// context and does not carry leases, grants, or other execution authority.
type WorkspaceFork struct {
	TenantID           uuid.UUID
	SourceWorkspaceID  uuid.UUID
	SourceGeneration   uint64
	ChildWorkspaceID   uuid.UUID
	ChildGeneration    uint64
	CreatedByPrincipal string
	CreatedAt          time.Time
}

type NewWorkspaceFork struct {
	TenantID           uuid.UUID
	SourceWorkspaceID  uuid.UUID
	SourceGeneration   uint64
	ChildWorkspaceID   uuid.UUID
	CreatedByPrincipal string
}

func (input NewWorkspaceFork) WorkspaceFork(now time.Time) (WorkspaceFork, error) {
	fork := WorkspaceFork{
		TenantID: input.TenantID, SourceWorkspaceID: input.SourceWorkspaceID,
		SourceGeneration: input.SourceGeneration, ChildWorkspaceID: input.ChildWorkspaceID,
		ChildGeneration: 1, CreatedByPrincipal: input.CreatedByPrincipal, CreatedAt: now.UTC(),
	}
	if err := fork.Validate(); err != nil {
		return WorkspaceFork{}, err
	}
	return fork, nil
}

func (fork WorkspaceFork) Validate() error {
	if fork.TenantID == uuid.Nil || fork.SourceWorkspaceID == uuid.Nil || fork.ChildWorkspaceID == uuid.Nil {
		return errors.New("fork tenant, source Workspace, and child Workspace IDs are required")
	}
	if fork.TenantID.Version() != 7 || fork.SourceWorkspaceID.Version() != 7 || fork.ChildWorkspaceID.Version() != 7 {
		return errors.New("fork tenant, source Workspace, and child Workspace IDs must be UUIDv7")
	}
	if fork.SourceWorkspaceID == fork.ChildWorkspaceID {
		return errors.New("fork child Workspace must differ from its source Workspace")
	}
	if fork.SourceGeneration < 1 || fork.SourceGeneration > math.MaxInt64 {
		return errors.New("fork source generation is outside the supported range")
	}
	if fork.ChildGeneration != 1 {
		return errors.New("fork child generation must be 1")
	}
	if !validBoundedSourceValue(fork.CreatedByPrincipal, maxGenerationCreatorIDLength) {
		return errors.New("fork creator is invalid")
	}
	if fork.CreatedAt.IsZero() {
		return errors.New("fork creation time is required")
	}
	return nil
}
