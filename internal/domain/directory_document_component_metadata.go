package domain

import (
	"errors"
	"time"

	"github.com/google/uuid"
)

// DirectoryComponentMetadata identifies the kind-specific record for a
// generic durable filesystem tree. Content and source state live separately.
type DirectoryComponentMetadata struct {
	TenantID    uuid.UUID
	WorkspaceID uuid.UUID
	ComponentID uuid.UUID
	CreatedAt   time.Time
}

type NewDirectoryComponentMetadata struct {
	TenantID    uuid.UUID
	WorkspaceID uuid.UUID
	ComponentID uuid.UUID
}

func (input NewDirectoryComponentMetadata) DirectoryComponentMetadata(now time.Time) (DirectoryComponentMetadata, error) {
	metadata := DirectoryComponentMetadata{
		TenantID: input.TenantID, WorkspaceID: input.WorkspaceID,
		ComponentID: input.ComponentID, CreatedAt: now.UTC(),
	}
	if err := validateComponentMetadataIdentity(
		metadata.TenantID, metadata.WorkspaceID, metadata.ComponentID,
		metadata.CreatedAt, "directory",
	); err != nil {
		return DirectoryComponentMetadata{}, err
	}
	return metadata, nil
}

// DocumentCollectionComponentMetadata identifies the kind-specific record for
// a logical document collection. Snapshot and source state live separately.
type DocumentCollectionComponentMetadata struct {
	TenantID    uuid.UUID
	WorkspaceID uuid.UUID
	ComponentID uuid.UUID
	CreatedAt   time.Time
}

type NewDocumentCollectionComponentMetadata struct {
	TenantID    uuid.UUID
	WorkspaceID uuid.UUID
	ComponentID uuid.UUID
}

func (input NewDocumentCollectionComponentMetadata) DocumentCollectionComponentMetadata(now time.Time) (DocumentCollectionComponentMetadata, error) {
	metadata := DocumentCollectionComponentMetadata{
		TenantID: input.TenantID, WorkspaceID: input.WorkspaceID,
		ComponentID: input.ComponentID, CreatedAt: now.UTC(),
	}
	if err := validateComponentMetadataIdentity(
		metadata.TenantID, metadata.WorkspaceID, metadata.ComponentID,
		metadata.CreatedAt, "document collection",
	); err != nil {
		return DocumentCollectionComponentMetadata{}, err
	}
	return metadata, nil
}

func validateComponentMetadataIdentity(tenantID, workspaceID, componentID uuid.UUID, createdAt time.Time, kind string) error {
	if tenantID == uuid.Nil || workspaceID == uuid.Nil || componentID == uuid.Nil {
		return errors.New(kind + " metadata tenant, workspace, and component IDs are required")
	}
	if tenantID.Version() != 7 || workspaceID.Version() != 7 || componentID.Version() != 7 {
		return errors.New(kind + " metadata tenant, workspace, and component IDs must be UUIDv7")
	}
	if createdAt.IsZero() {
		return errors.New(kind + " component metadata creation time is required")
	}
	return nil
}
