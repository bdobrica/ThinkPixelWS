package domain

import (
	"errors"
	"math"

	"github.com/google/uuid"
)

const maxTaintLength = 128

// ComponentClassificationMetadata is the effective policy metadata for one
// component in an immutable Workspace generation. It is policy input and does
// not itself grant access or authority.
type ComponentClassificationMetadata struct {
	TenantID       uuid.UUID
	WorkspaceID    uuid.UUID
	Generation     uint64
	ComponentID    uuid.UUID
	Classification Classification
	Taints         []string
}

type NewComponentClassificationMetadata struct {
	TenantID       uuid.UUID
	WorkspaceID    uuid.UUID
	Generation     uint64
	ComponentID    uuid.UUID
	Classification Classification
	Taints         []string
}

func (input NewComponentClassificationMetadata) ComponentClassificationMetadata() (ComponentClassificationMetadata, error) {
	metadata := ComponentClassificationMetadata{
		TenantID: input.TenantID, WorkspaceID: input.WorkspaceID,
		Generation: input.Generation, ComponentID: input.ComponentID,
		Classification: input.Classification, Taints: append([]string(nil), input.Taints...),
	}
	if err := metadata.Validate(); err != nil {
		return ComponentClassificationMetadata{}, err
	}
	return metadata, nil
}

func (metadata ComponentClassificationMetadata) Validate() error {
	if metadata.TenantID == uuid.Nil || metadata.WorkspaceID == uuid.Nil || metadata.ComponentID == uuid.Nil {
		return errors.New("component classification tenant, workspace, and component IDs are required")
	}
	if metadata.TenantID.Version() != 7 || metadata.WorkspaceID.Version() != 7 || metadata.ComponentID.Version() != 7 {
		return errors.New("component classification tenant, workspace, and component IDs must be UUIDv7")
	}
	if metadata.Generation < 1 || metadata.Generation > math.MaxInt64 {
		return errors.New("component classification generation is outside the supported range")
	}
	if !metadata.Classification.valid() {
		return errors.New("component classification is invalid")
	}
	return validateTaints(metadata.Taints)
}

func validateTaints(taints []string) error {
	seen := make(map[string]struct{}, len(taints))
	for _, taint := range taints {
		if !validBoundedSourceValue(taint, maxTaintLength) {
			return errors.New("taint is invalid")
		}
		if _, exists := seen[taint]; exists {
			return errors.New("taints must be unique")
		}
		seen[taint] = struct{}{}
	}
	return nil
}
