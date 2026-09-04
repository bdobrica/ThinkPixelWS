package domain

import (
	"errors"
	"time"

	"github.com/google/uuid"
)

type WorkspaceComponentKind string

const (
	WorkspaceComponentRepository         WorkspaceComponentKind = "repository"
	WorkspaceComponentDirectory          WorkspaceComponentKind = "directory"
	WorkspaceComponentDocumentCollection WorkspaceComponentKind = "document-collection"
	WorkspaceComponentArtifactCollection WorkspaceComponentKind = "artifact-collection"
)

// WorkspaceComponent is a stable identity within a Workspace. Its immutable
// generation state, kind-specific metadata, and provenance are stored
// separately.
type WorkspaceComponent struct {
	TenantID      uuid.UUID
	WorkspaceID   uuid.UUID
	ID            uuid.UUID
	Name          string
	Kind          WorkspaceComponentKind
	CanonicalPath string
	CreatedAt     time.Time
}

type NewWorkspaceComponent struct {
	TenantID    uuid.UUID
	WorkspaceID uuid.UUID
	ID          uuid.UUID
	Name        string
	Kind        WorkspaceComponentKind
}

func (input NewWorkspaceComponent) WorkspaceComponent(now time.Time) (WorkspaceComponent, error) {
	component := WorkspaceComponent{
		TenantID: input.TenantID, WorkspaceID: input.WorkspaceID, ID: input.ID,
		Name: input.Name, Kind: input.Kind, CanonicalPath: "/workspace/" + input.Name,
		CreatedAt: now.UTC(),
	}
	if err := component.Validate(); err != nil {
		return WorkspaceComponent{}, err
	}
	return component, nil
}

func (component WorkspaceComponent) Validate() error {
	if component.TenantID == uuid.Nil || component.WorkspaceID == uuid.Nil || component.ID == uuid.Nil {
		return errors.New("tenant, workspace, and component IDs are required")
	}
	if component.TenantID.Version() != 7 || component.WorkspaceID.Version() != 7 || component.ID.Version() != 7 {
		return errors.New("tenant, workspace, and component IDs must be UUIDv7")
	}
	if !workspaceNamePattern.MatchString(component.Name) {
		return errors.New("workspace component name has invalid format")
	}
	if !component.Kind.valid() {
		return errors.New("workspace component kind is invalid")
	}
	if component.CanonicalPath != "/workspace/"+component.Name {
		return errors.New("workspace component path is not canonical")
	}
	if component.CreatedAt.IsZero() {
		return errors.New("workspace component creation time is required")
	}
	return nil
}

func (kind WorkspaceComponentKind) valid() bool {
	switch kind {
	case WorkspaceComponentRepository, WorkspaceComponentDirectory,
		WorkspaceComponentDocumentCollection, WorkspaceComponentArtifactCollection:
		return true
	default:
		return false
	}
}
